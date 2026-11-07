package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/controlplane"
	"github.com/FernandoVillegas27/ai-engineering-os/core/registry"
	"github.com/FernandoVillegas27/ai-engineering-os/core/runstate"
	"github.com/FernandoVillegas27/ai-engineering-os/core/toolapproval"
)

type runToolApprovalOptions struct {
	runID       string
	taskID      string
	toolID      string
	operation   string
	stateDir    string
	eventsDir   string
	approvalDir string
	expiresIn   time.Duration
	json        bool
}

type toolApprovalView struct {
	RunID      string     `json:"run_id"`
	TaskID     string     `json:"task_id"`
	ToolID     string     `json:"tool_id"`
	Operation  string     `json:"operation"`
	Status     string     `json:"status"`
	EvidenceID string     `json:"evidence_id,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
}

func runRunToolApproval(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("uso: ai run tool-approval <request|grant|reject> ...")
	}
	switch args[0] {
	case "request":
		return runRunToolApprovalRequest(args[1:])
	case "grant":
		return runRunToolApprovalResolve(args[1:], true)
	case "reject":
		return runRunToolApprovalResolve(args[1:], false)
	default:
		return fmt.Errorf("subcomando tool-approval desconocido: %s", args[0])
	}
}

func runRunToolApprovalRequest(args []string) error {
	options, err := parseRunToolApprovalArgs(args, false)
	if err != nil {
		return err
	}
	run, tool, err := validateToolApprovalTarget(options)
	if err != nil {
		return err
	}
	if !registry.OperationRequiresApproval(tool, options.operation) {
		return fmt.Errorf("tool %q operation %q does not require approval", options.toolID, options.operation)
	}
	events, eventStore, err := loadToolApprovalEvents(options)
	if err != nil {
		return err
	}
	if toolApprovalPending(events, options.taskID, options.toolID, options.operation) {
		return fmt.Errorf("approval is already pending for %s/%s on task %s", options.toolID, options.operation, options.taskID)
	}
	event, err := controlplane.NewEvent(controlplane.NewEventRequest{
		RunID:   run.ID,
		Type:    controlplane.EventApprovalRequested,
		TaskID:  options.taskID,
		Summary: fmt.Sprintf("%s/%s requires human approval", options.toolID, options.operation),
		Data: map[string]any{
			"kind":      "tool-operation",
			"tool_id":   options.toolID,
			"operation": options.operation,
		},
	}, time.Now().UTC())
	if err != nil {
		return err
	}
	if _, err := eventStore.Append(context.Background(), event); err != nil {
		return err
	}
	store, err := openToolApprovalStore(options.approvalDir)
	if err != nil {
		return err
	}
	if err := store.Save(evidence); err != nil {
		return fmt.Errorf("persist approval evidence after audit event: %w", err)
	}
	return printToolApprovalView(toolApprovalView{
		RunID: run.ID, TaskID: options.taskID, ToolID: options.toolID,
		Operation: options.operation, Status: "pending",
	}, options.json)
}

func runRunToolApprovalResolve(args []string, granted bool) error {
	options, err := parseRunToolApprovalArgs(args, true)
	if err != nil {
		return err
	}
	run, tool, err := validateToolApprovalTarget(options)
	if err != nil {
		return err
	}
	if !registry.OperationRequiresApproval(tool, options.operation) {
		return fmt.Errorf("tool %q operation %q does not require approval", options.toolID, options.operation)
	}
	events, eventStore, err := loadToolApprovalEvents(options)
	if err != nil {
		return err
	}
	if !toolApprovalPending(events, options.taskID, options.toolID, options.operation) {
		return fmt.Errorf("no pending approval for %s/%s on task %s", options.toolID, options.operation, options.taskID)
	}

	status := toolapproval.StatusRejected
	eventType := controlplane.EventApprovalRejected
	summary := fmt.Sprintf("%s/%s rejected by human", options.toolID, options.operation)
	if granted {
		status = toolapproval.StatusGranted
		eventType = controlplane.EventApprovalGranted
		summary = fmt.Sprintf("%s/%s granted by human", options.toolID, options.operation)
	}

	now := time.Now().UTC()
	var expiresAt *time.Time
	if granted && options.expiresIn > 0 {
		value := now.Add(options.expiresIn).UTC()
		expiresAt = &value
	}
	evidence, err := toolapproval.New(toolapproval.Request{
		RunID: run.ID, TaskID: options.taskID, ToolID: options.toolID,
		Operation: options.operation, Status: status, Actor: "human", ExpiresAt: expiresAt,
	}, now)
	if err != nil {
		return err
	}
	event, err := controlplane.NewEvent(controlplane.NewEventRequest{
		RunID:   run.ID,
		Type:    eventType,
		TaskID:  options.taskID,
		Summary: summary,
		Data: map[string]any{
			"kind":        "tool-operation",
			"tool_id":     options.toolID,
			"operation":   options.operation,
			"evidence_id": evidence.ID,
			"expires_at":  expiresAt,
		},
	}, now)
	if err != nil {
		return err
	}
	if _, err := eventStore.Append(context.Background(), event); err != nil {
		return err
	}
	return printToolApprovalView(toolApprovalView{
		RunID: run.ID, TaskID: options.taskID, ToolID: options.toolID,
		Operation: options.operation, Status: string(status),
		EvidenceID: evidence.ID, ExpiresAt: expiresAt,
	}, options.json)
}

func parseRunToolApprovalArgs(args []string, resolving bool) (runToolApprovalOptions, error) {
	var options runToolApprovalOptions
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--task":
			if i+1 >= len(args) { return options, fmt.Errorf("--task requiere un id") }
			i++; options.taskID = args[i]
		case "--tool":
			if i+1 >= len(args) { return options, fmt.Errorf("--tool requiere un id") }
			i++; options.toolID = args[i]
		case "--operation":
			if i+1 >= len(args) { return options, fmt.Errorf("--operation requiere un valor") }
			i++; options.operation = args[i]
		case "--state-dir":
			if i+1 >= len(args) { return options, fmt.Errorf("--state-dir requiere una ruta") }
			i++; options.stateDir = args[i]
		case "--events-dir":
			if i+1 >= len(args) { return options, fmt.Errorf("--events-dir requiere una ruta") }
			i++; options.eventsDir = args[i]
		case "--approval-dir":
			if i+1 >= len(args) { return options, fmt.Errorf("--approval-dir requiere una ruta") }
			i++; options.approvalDir = args[i]
		case "--expires-in":
			if !resolving {
				return options, fmt.Errorf("--expires-in solo es válido al resolver una aprobación")
			}
			if i+1 >= len(args) { return options, fmt.Errorf("--expires-in requiere una duración") }
			i++
			value, err := time.ParseDuration(args[i])
			if err != nil || value <= 0 { return options, fmt.Errorf("--expires-in debe ser una duración positiva") }
			options.expiresIn = value
		case "--json":
			options.json = true
		default:
			if strings.HasPrefix(args[i], "-") { return options, fmt.Errorf("opción desconocida: %s", args[i]) }
			if options.runID != "" { return options, fmt.Errorf("se esperaba un solo run id") }
			options.runID = args[i]
		}
	}
	if strings.TrimSpace(options.runID) == "" || strings.TrimSpace(options.taskID) == "" ||
		strings.TrimSpace(options.toolID) == "" || strings.TrimSpace(options.operation) == "" {
		return options, fmt.Errorf("run id, --task, --tool y --operation son obligatorios")
	}
	return options, nil
}

func validateToolApprovalTarget(options runToolApprovalOptions) (runstate.Run, registry.Tool, error) {
	runStore, err := openRunStore(options.stateDir)
	if err != nil { return runstate.Run{}, registry.Tool{}, err }
	run, err := runStore.Load(options.runID)
	if err != nil { return runstate.Run{}, registry.Tool{}, err }

	var unit *runstate.WorkUnit
	for i := range run.WorkUnits {
		if run.WorkUnits[i].ID == options.taskID {
			unit = &run.WorkUnits[i]
			break
		}
	}
	if unit == nil {
		return runstate.Run{}, registry.Tool{}, fmt.Errorf("task %s does not exist in run %s", options.taskID, run.ID)
	}
	declared := false
	for _, id := range unit.Tools {
		if id == options.toolID {
			declared = true
			break
		}
	}
	if !declared {
		return runstate.Run{}, registry.Tool{}, fmt.Errorf("tool %q is not declared for task %s", options.toolID, options.taskID)
	}

	toolsPath, err := resolveRegistryFile("tools.yaml")
	if err != nil { return runstate.Run{}, registry.Tool{}, err }
	tools, err := registry.LoadTools(toolsPath)
	if err != nil { return runstate.Run{}, registry.Tool{}, err }
	tool, ok := tools.Tools[options.toolID]
	if !ok || tool.Status != "enabled" {
		return runstate.Run{}, registry.Tool{}, fmt.Errorf("tool %q is not enabled in the Tool Registry", options.toolID)
	}
	operationDeclared := false
	for _, operation := range tool.Operations {
		if operation == options.operation {
			operationDeclared = true
			break
		}
	}
	if !operationDeclared {
		return runstate.Run{}, registry.Tool{}, fmt.Errorf("operation %q is not declared for tool %q", options.operation, options.toolID)
	}
	return run, tool, nil
}

func loadToolApprovalEvents(options runToolApprovalOptions) ([]controlplane.Event, *controlplane.EventStore, error) {
	store, err := openControlPlaneStore(options.eventsDir)
	if err != nil { return nil, nil, err }
	events, err := store.List(options.runID)
	if err != nil { return nil, nil, err }
	if len(events) == 0 {
		return nil, nil, fmt.Errorf("no control-plane events found for run %s", options.runID)
	}
	return events, store, nil
}

func toolApprovalPending(events []controlplane.Event, taskID, toolID, operation string) bool {
	pending := false
	for _, event := range events {
		if event.TaskID != taskID { continue }
		var data struct {
			Kind string `json:"kind"`
			ToolID string `json:"tool_id"`
			Operation string `json:"operation"`
		}
		if err := jsonUnmarshalApprovalData(event, &data); err != nil { continue }
		if data.Kind != "tool-operation" || data.ToolID != toolID || data.Operation != operation { continue }
		switch event.Type {
		case controlplane.EventApprovalRequested:
			pending = true
		case controlplane.EventApprovalGranted, controlplane.EventApprovalRejected:
			pending = false
		}
	}
	return pending
}

func toolApprovalResolved(events []controlplane.Event, taskID, toolID, operation string) bool {
	for _, event := range events {
		if event.TaskID != taskID { continue }
		if event.Type != controlplane.EventApprovalGranted && event.Type != controlplane.EventApprovalRejected { continue }
		var data struct {
			Kind string `json:"kind"`
			ToolID string `json:"tool_id"`
			Operation string `json:"operation"`
		}
		if err := jsonUnmarshalApprovalData(event, &data); err != nil { continue }
		if data.Kind == "tool-operation" && data.ToolID == toolID && data.Operation == operation {
			return true
		}
	}
	return false
}

func jsonUnmarshalApprovalData(event controlplane.Event, target any) error {
	return json.Unmarshal(event.Data, target)
}

func openToolApprovalStore(explicit string) (*toolapproval.Store, error) {
	root, err := resolveToolApprovalDir(explicit)
	if err != nil { return nil, err }
	return toolapproval.NewStore(root)
}

func resolveToolApprovalDir(explicit string) (string, error) {
	if strings.TrimSpace(explicit) != "" { return explicit, nil }
	if value := strings.TrimSpace(os.Getenv("AI_ENGINEERING_OS_APPROVALS_DIR")); value != "" {
		return value, nil
	}
	configRoot, err := os.UserConfigDir()
	if err != nil { return "", fmt.Errorf("resolver directorio de configuración: %w", err) }
	return filepath.Join(configRoot, "ai-engineering-os", "tool-approvals"), nil
}

func printToolApprovalView(view toolApprovalView, asJSON bool) error {
	if asJSON { return printJSON(view) }
	fmt.Println("Tool Operation Approval")
	fmt.Printf("Run: %s\n", view.RunID)
	fmt.Printf("Task: %s\n", view.TaskID)
	fmt.Printf("Operation: %s/%s\n", view.ToolID, view.Operation)
	fmt.Printf("Status: %s\n", view.Status)
	if view.EvidenceID != "" { fmt.Printf("Evidence: %s\n", view.EvidenceID) }
	if view.ExpiresAt != nil { fmt.Printf("Expires: %s\n", view.ExpiresAt.Format(time.RFC3339)) }
	return nil
}
