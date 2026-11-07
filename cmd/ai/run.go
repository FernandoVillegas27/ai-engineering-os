package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/controlplane"
	"github.com/FernandoVillegas27/ai-engineering-os/core/runstate"
)

type runCommandOptions struct {
	stateDir  string
	eventsDir string
	json      bool
}

func runRun(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("uso: ai run <init|status|list|transition|control|events> ...")
	}
	switch args[0] {
	case "init":
		return runRunInit(args[1:])
	case "status":
		return runRunStatus(args[1:])
	case "list":
		return runRunList(args[1:])
	case "transition":
		return runRunTransition(args[1:])
	case "control":
		return runRunControl(args[1:])
	case "events":
		return runRunEvents(args[1:])
	case "execute":
		return runRunExecute(args[1:])
	case "serve":
		return runRunServe(args[1:])
	case "promotion":
		return runRunPromotion(args[1:])
	case "promote":
		return runRunPromote(args[1:])
	case "cancel":
		return runRunCancel(args[1:])
	case "retry":
		return runRunRetry(args[1:])
	case "pause":
		return runRunPause(args[1:])
	case "resume":
		return runRunResume(args[1:])
	case "recover":
		return runRunRecover(args[1:])
	case "snapshot":
		return runRunSnapshot(args[1:])
	case "archive":
		return runRunArchive(args[1:])
	case "compact":
		return runRunCompact(args[1:])
	case "tool-approval":
		return runRunToolApproval(args[1:])
	default:
		return fmt.Errorf("subcomando run desconocido: %s", args[0])
	}
}

func runRunInit(args []string) error {
	options := runCommandOptions{}
	var task, projectID, projectRoot string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--state-dir":
			if i+1 >= len(args) {
				return fmt.Errorf("--state-dir requiere una ruta")
			}
			i++
			options.stateDir = args[i]
		case "--task":
			if i+1 >= len(args) {
				return fmt.Errorf("--task requiere texto")
			}
			i++
			task = args[i]
		case "--project":
			if i+1 >= len(args) {
				return fmt.Errorf("--project requiere un id")
			}
			i++
			projectID = args[i]
		case "--project-root":
			if i+1 >= len(args) {
				return fmt.Errorf("--project-root requiere una ruta")
			}
			i++
			projectRoot = args[i]
		case "--json":
			options.json = true
		case "-h", "--help":
			printRunUsage()
			return nil
		default:
			return fmt.Errorf("opción desconocida: %s", args[i])
		}
	}
	if strings.TrimSpace(task) == "" || strings.TrimSpace(projectID) == "" {
		return fmt.Errorf("--task y --project son obligatorios")
	}
	if strings.TrimSpace(projectRoot) != "" {
		absolute, err := filepath.Abs(projectRoot)
		if err != nil {
			return fmt.Errorf("resolver project root: %w", err)
		}
		projectRoot = absolute
	}

	store, err := openRunStore(options.stateDir)
	if err != nil {
		return err
	}
	run, err := runstate.New(runstate.NewRunRequest{
		Task:        task,
		ProjectID:   projectID,
		ProjectRoot: projectRoot,
	}, time.Now().UTC())
	if err != nil {
		return err
	}
	if err := store.Save(run); err != nil {
		return err
	}
	return printRunRecord(run, options.json)
}

func runRunStatus(args []string) error {
	options := runCommandOptions{}
	var id string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--state-dir":
			if i+1 >= len(args) {
				return fmt.Errorf("--state-dir requiere una ruta")
			}
			i++
			options.stateDir = args[i]
		case "--json":
			options.json = true
		case "-h", "--help":
			printRunUsage()
			return nil
		default:
			if strings.HasPrefix(args[i], "-") {
				return fmt.Errorf("opción desconocida: %s", args[i])
			}
			if id != "" {
				return fmt.Errorf("se esperaba un solo run id")
			}
			id = args[i]
		}
	}
	if id == "" {
		return fmt.Errorf("run id es obligatorio")
	}
	store, err := openRunStore(options.stateDir)
	if err != nil {
		return err
	}
	run, err := store.Load(id)
	if err != nil {
		return err
	}
	return printRunRecord(run, options.json)
}

func runRunList(args []string) error {
	options := runCommandOptions{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--state-dir":
			if i+1 >= len(args) {
				return fmt.Errorf("--state-dir requiere una ruta")
			}
			i++
			options.stateDir = args[i]
		case "--json":
			options.json = true
		case "-h", "--help":
			printRunUsage()
			return nil
		default:
			return fmt.Errorf("opción desconocida: %s", args[i])
		}
	}
	store, err := openRunStore(options.stateDir)
	if err != nil {
		return err
	}
	runs, err := store.List()
	if err != nil {
		return err
	}
	if options.json {
		return printJSON(runs)
	}
	if len(runs) == 0 {
		fmt.Println("No durable runs found.")
		return nil
	}
	for _, run := range runs {
		fmt.Printf("%s  %-10s  %s  project=%s\n", run.ID, run.Status, run.Task, run.ProjectID)
	}
	return nil
}

func runRunTransition(args []string) error {
	options := runCommandOptions{}
	var id string
	var status runstate.Status
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--state-dir":
			if i+1 >= len(args) {
				return fmt.Errorf("--state-dir requiere una ruta")
			}
			i++
			options.stateDir = args[i]
		case "--status":
			if i+1 >= len(args) {
				return fmt.Errorf("--status requiere un valor")
			}
			i++
			status = runstate.Status(args[i])
		case "--json":
			options.json = true
		case "-h", "--help":
			printRunUsage()
			return nil
		default:
			if strings.HasPrefix(args[i], "-") {
				return fmt.Errorf("opción desconocida: %s", args[i])
			}
			if id != "" {
				return fmt.Errorf("se esperaba un solo run id")
			}
			id = args[i]
		}
	}
	if id == "" || status == "" {
		return fmt.Errorf("run id y --status son obligatorios")
	}
	store, err := openRunStore(options.stateDir)
	if err != nil {
		return err
	}
	run, err := store.Load(id)
	if err != nil {
		return err
	}
	if err := runstate.Transition(&run, status, time.Now().UTC()); err != nil {
		return err
	}
	if err := store.Save(run); err != nil {
		return err
	}
	return printRunRecord(run, options.json)
}

func runRunControl(args []string) error {
	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
		printRunUsage()
		return nil
	}
	options := runCommandOptions{}
	id, err := parseControlPlaneArgs(args, &options)
	if err != nil {
		return err
	}
	store, err := openControlPlaneStore(options.eventsDir)
	if err != nil {
		return err
	}
	events, err := store.List(id)
	if err != nil {
		return err
	}
	if len(events) == 0 {
		return fmt.Errorf("no control-plane events found for run %s", id)
	}
	state, err := controlplane.Project(events)
	if err != nil {
		return err
	}
	if options.json {
		return printJSON(state)
	}
	printControlPlaneState(state)
	return nil
}

func runRunEvents(args []string) error {
	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
		printRunUsage()
		return nil
	}
	options := runCommandOptions{}
	id, err := parseControlPlaneArgs(args, &options)
	if err != nil {
		return err
	}
	store, err := openControlPlaneStore(options.eventsDir)
	if err != nil {
		return err
	}
	events, err := store.List(id)
	if err != nil {
		return err
	}
	if options.json {
		return printJSON(events)
	}
	if len(events) == 0 {
		fmt.Printf("No control-plane events found for run %s.\n", id)
		return nil
	}
	for _, event := range events {
		parts := []string{fmt.Sprintf("#%d", event.Sequence), string(event.Type)}
		if event.AgentID != "" {
			parts = append(parts, "agent="+event.AgentID)
		}
		if event.TaskID != "" {
			parts = append(parts, "task="+event.TaskID)
		}
		if event.Summary != "" {
			parts = append(parts, event.Summary)
		}
		fmt.Println(strings.Join(parts, "  "))
	}
	return nil
}

func parseControlPlaneArgs(args []string, options *runCommandOptions) (string, error) {
	var id string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--events-dir":
			if i+1 >= len(args) {
				return "", fmt.Errorf("--events-dir requiere una ruta")
			}
			i++
			options.eventsDir = args[i]
		case "--json":
			options.json = true
		case "-h", "--help":
			return "", fmt.Errorf("--help debe usarse sin otros argumentos")
		default:
			if strings.HasPrefix(args[i], "-") {
				return "", fmt.Errorf("opción desconocida: %s", args[i])
			}
			if id != "" {
				return "", fmt.Errorf("se esperaba un solo run id")
			}
			id = args[i]
		}
	}
	if id == "" {
		return "", fmt.Errorf("run id es obligatorio")
	}
	return id, nil
}

func openControlPlaneStore(explicit string) (*controlplane.EventStore, error) {
	root, err := resolveControlPlaneDir(explicit)
	if err != nil {
		return nil, err
	}
	return controlplane.NewEventStore(root)
}

func resolveControlPlaneDir(explicit string) (string, error) {
	if strings.TrimSpace(explicit) != "" {
		return explicit, nil
	}
	if value := strings.TrimSpace(os.Getenv("AI_ENGINEERING_OS_EVENTS_DIR")); value != "" {
		return value, nil
	}
	configRoot, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolver directorio de configuración: %w", err)
	}
	return filepath.Join(configRoot, "ai-engineering-os", "events"), nil
}

func printControlPlaneState(state controlplane.State) {
	fmt.Println("Multi-Agent Control Plane")
	fmt.Printf("Run: %s\n", state.RunID)
	fmt.Printf("Status: %s\n", state.Status)
	fmt.Printf("Last event: #%d\n", state.LastSequence)
	fmt.Printf(
		"Progress: total=%d queued=%d assigned=%d running=%d waiting=%d blocked=%d completed=%d failed=%d cancelled=%d\n",
		state.Progress.Total,
		state.Progress.Queued,
		state.Progress.Assigned,
		state.Progress.Running,
		state.Progress.Waiting,
		state.Progress.Blocked,
		state.Progress.Completed,
		state.Progress.Failed,
		state.Progress.Cancelled,
	)

	if len(state.Agents) > 0 {
		fmt.Println("Agents:")
		ids := make([]string, 0, len(state.Agents))
		for id := range state.Agents {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			agent := state.Agents[id]
			active := "-"
			if len(agent.ActiveTasks) > 0 {
				active = strings.Join(agent.ActiveTasks, ",")
			}
			fmt.Printf("  - %s [%s] active=%s tasks=%d\n", agent.ID, agent.Status, active, agent.TaskCount)
		}
	}

	if len(state.Tasks) > 0 {
		fmt.Println("Tasks:")
		ids := make([]string, 0, len(state.Tasks))
		for id := range state.Tasks {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			task := state.Tasks[id]
			agent := task.AgentID
			if agent == "" {
				agent = "-"
			}
			fmt.Printf("  - %s [%s] agent=%s\n", task.ID, task.Status, agent)
		}
	}

	fmt.Printf(
		"Quality: verification=%s review=%s baseline=%d corrections=%d\n",
		emptyDash(state.Quality.Verification),
		emptyDash(state.Quality.Review),
		state.Quality.BaselineComparisons,
		state.Quality.Corrections,
	)
	fmt.Printf(
		"Coordination: messages=%d artifacts=%d pending-approvals=%d\n",
		state.MessageCount,
		state.ArtifactCount,
		state.PendingApprovals,
	)
	if state.Runtime != "" || state.Model != "" {
		fmt.Printf("Runtime: %s  Model: %s\n", emptyDash(state.Runtime), emptyDash(state.Model))
	}
}

func emptyDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

func openRunStore(explicit string) (*runstate.Store, error) {
	root, err := resolveRunStateDir(explicit)
	if err != nil {
		return nil, err
	}
	return runstate.NewStore(root)
}

func resolveRunStateDir(explicit string) (string, error) {
	if strings.TrimSpace(explicit) != "" {
		return explicit, nil
	}
	if value := strings.TrimSpace(os.Getenv("AI_ENGINEERING_OS_STATE_DIR")); value != "" {
		return value, nil
	}
	configRoot, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolver directorio de configuración: %w", err)
	}
	return filepath.Join(configRoot, "ai-engineering-os", "runs"), nil
}

func printRunRecord(run runstate.Run, asJSON bool) error {
	if asJSON {
		return printJSON(run)
	}
	fmt.Println("Durable Run")
	fmt.Printf("ID: %s\n", run.ID)
	fmt.Printf("Task ID: %s\n", run.TaskID)
	fmt.Printf("Status: %s\n", run.Status)
	fmt.Printf("Project: %s\n", run.ProjectID)
	fmt.Printf("Task: %s\n", run.Task)
	fmt.Printf("Updated: %s\n", run.UpdatedAt.Format(time.RFC3339))
	if len(run.WorkUnits) > 0 {
		fmt.Println("Agents:")
		for _, unit := range run.WorkUnits {
			fmt.Printf("  - %s [%s] %s\n", unit.AgentID, unit.Status, unit.ID)
		}
	}
	if run.Quality != nil {
		fmt.Printf("Quality: passed=%t reason=%s\n", run.Quality.Passed, run.Quality.Reason)
	}
	return nil
}

func printJSON(value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

func printRunUsage() {
	fmt.Println("Uso:")
	fmt.Println("  ai run init --task <tarea> --project <id> [--project-root ruta] [--state-dir ruta] [--json]")
	fmt.Println("  ai run status <run-id> [--state-dir ruta] [--json]")
	fmt.Println("  ai run list [--state-dir ruta] [--json]")
	fmt.Println("  ai run transition <run-id> --status planned|running|paused|completed|failed|cancelled [--state-dir ruta] [--json]")
	fmt.Println("  ai run control <run-id> [--events-dir ruta] [--json]")
	fmt.Println("  ai run events <run-id> [--events-dir ruta] [--json]")
	fmt.Println("  ai run execute [ruta] --task <tarea> (--dry-run | --runtime codex) [--project id] [--max-concurrency N] [--ai] [--json]")
	fmt.Println("  ai run execute [ruta] --task <tarea> --runtime auto|codex|cursor --multi-agent-write --trust-project-code [--memory auto|engram|off] [--model-policy auto|economy|balanced|deep] [--model id] [--max-concurrency N]")
	fmt.Println("  ai run serve [--addr 127.0.0.1:8765] [--events-dir ruta]")
	fmt.Println("  ai run promotion <run-id> [--promotion-dir ruta] [--json]")
	fmt.Println("  ai run promote <run-id> --approve [--promotion-dir ruta] [--state-dir ruta] [--events-dir ruta] [--json]")
	fmt.Println("  ai run cancel <run-id> [--control-dir ruta] [--state-dir ruta] [--events-dir ruta] [--json]")
	fmt.Println("  ai run retry <run-id> <task-id> [--state-dir ruta] [--events-dir ruta] [--artifacts-dir ruta] [--messages-dir ruta] [--control-dir ruta] [--json]")
	fmt.Println("  ai run pause <run-id> [--control-dir ruta] [--state-dir ruta] [--events-dir ruta] [--json]")
	fmt.Println("  ai run resume <run-id> [--trust-project-code] [--state-dir ruta] [--events-dir ruta] [--artifacts-dir ruta] [--messages-dir ruta] [--control-dir ruta] [--checkpoint-dir ruta] [--promotion-dir ruta] [--max-concurrency N] [--json]")
	fmt.Println("  ai run recover <run-id> [--stale-after 30s] [--state-dir ruta] [--events-dir ruta] [--control-dir ruta] [--checkpoint-dir ruta] [--json]")
	fmt.Println("  ai run snapshot <run-id> [--events-dir ruta] [--snapshot-dir ruta] [--json]")
	fmt.Println("  ai run archive <run-id> [--events-dir ruta] [--snapshot-dir ruta] [--archive-dir ruta] [--json]")
	fmt.Println("  ai run compact <run-id> [--events-dir ruta] [--snapshot-dir ruta] [--archive-dir ruta] [--json]")
	fmt.Println("  ai run tool-approval request <run-id> --task <task-id> --tool <tool-id> --operation <operation> [--request-digest sha256] [--state-dir ruta] [--events-dir ruta] [--json]")
	fmt.Println("  ai run tool-approval grant <run-id> --task <task-id> --tool <tool-id> --operation <operation> [--request-digest sha256] [--expires-in 15m] [--approval-dir ruta] [--state-dir ruta] [--events-dir ruta] [--json]")
	fmt.Println("  ai run tool-approval reject <run-id> --task <task-id> --tool <tool-id> --operation <operation> [--request-digest sha256] [--approval-dir ruta] [--state-dir ruta] [--events-dir ruta] [--json]")
	fmt.Println()
	fmt.Println("Sin --state-dir usa AI_ENGINEERING_OS_STATE_DIR o el directorio de configuración del usuario.")
	fmt.Println("Sin --events-dir usa AI_ENGINEERING_OS_EVENTS_DIR o el directorio de configuración del usuario.")
}
