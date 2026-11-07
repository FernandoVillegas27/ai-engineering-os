package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/artifact"
	"github.com/FernandoVillegas27/ai-engineering-os/core/collaboration"
	"github.com/FernandoVillegas27/ai-engineering-os/core/controlplane"
	"github.com/FernandoVillegas27/ai-engineering-os/core/coordinator"
	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/message"
	"github.com/FernandoVillegas27/ai-engineering-os/core/modelpolicy"
	"github.com/FernandoVillegas27/ai-engineering-os/core/orchestration"
	"github.com/FernandoVillegas27/ai-engineering-os/core/runstate"
	"github.com/FernandoVillegas27/ai-engineering-os/core/scheduler"
)

type runRetryOptions struct {
	runID        string
	taskID       string
	stateDir     string
	eventsDir    string
	artifactsDir string
	messagesDir  string
	controlDir   string
	json         bool
}

func runRunRetry(args []string) error {
	options, err := parseRunRetryArgs(args)
	if err != nil {
		return err
	}

	runStore, err := openRunStore(options.stateDir)
	if err != nil {
		return err
	}
	parent, err := runStore.Load(options.runID)
	if err != nil {
		return fmt.Errorf("load parent run: %w", err)
	}
	if parent.Status != runstate.StatusFailed {
		return fmt.Errorf("run %s is not failed: status=%s", parent.ID, parent.Status)
	}
	if strings.TrimSpace(parent.Intent) == "" {
		return fmt.Errorf("run %s has no durable intent and cannot be retried safely", parent.ID)
	}
	for _, unit := range parent.WorkUnits {
		if unit.Mutating {
			return fmt.Errorf("run %s contains mutating work; targeted retry requires a future integration checkpoint", parent.ID)
		}
	}

	target, err := retryTarget(parent, options.taskID)
	if err != nil {
		return err
	}
	if err := validateRetryDependencies(parent, target); err != nil {
		return err
	}

	eventStore, err := openControlPlaneStore(options.eventsDir)
	if err != nil {
		return err
	}
	parentEvents, err := eventStore.List(parent.ID)
	if err != nil {
		return err
	}
	if len(parentEvents) == 0 {
		return fmt.Errorf("parent run %s has no Control Plane event stream", parent.ID)
	}
	parentState, err := controlplane.Project(parentEvents)
	if err != nil {
		return fmt.Errorf("project parent Control Plane state: %w", err)
	}
	if parentState.Status != controlplane.RunFailed {
		return fmt.Errorf(
			"parent Control Plane state is %s; expected failed before targeted retry",
			parentState.Status,
		)
	}

	artifactsRoot, err := resolveRunAuxDir(options.artifactsDir, "AI_ENGINEERING_OS_ARTIFACTS_DIR", "artifacts")
	if err != nil {
		return err
	}
	artifactStore, err := artifact.NewStore(artifactsRoot)
	if err != nil {
		return err
	}
	messagesRoot, err := resolveRunAuxDir(options.messagesDir, "AI_ENGINEERING_OS_MESSAGES_DIR", "messages")
	if err != nil {
		return err
	}
	messageBus, err := message.NewBus(messagesRoot)
	if err != nil {
		return err
	}

	unit := retryExecutionUnit(parent, target)
	contextProvider := collaboration.InboxContextProvider{
		RunID:         parent.ID,
		MessageBus:    messageBus,
		ArtifactStore: artifactStore,
	}

	var executor execution.Executor
	switch parent.Runtime.Provider {
	case "dry-run":
		executor = execution.DryRunExecutor{}
	case "auto", "codex", "cursor":
		policy := modelpolicy.PolicyAuto
		if strings.TrimSpace(parent.Runtime.ModelPolicy) != "" {
			policy, err = modelpolicy.Parse(parent.Runtime.ModelPolicy)
			if err != nil {
				return fmt.Errorf("parse parent model policy: %w", err)
			}
		}
		runtime, _, runtimeErr := executionRuntimeFor(
			parent.Runtime.Provider,
			modelRoutingOptions{Policy: policy, Override: parent.Runtime.Model},
		)
		if runtimeErr != nil {
			return runtimeErr
		}
		executor = execution.RuntimeExecutor{Runtime: runtime}
	default:
		return fmt.Errorf("retry does not support parent runtime %q", parent.Runtime.Provider)
	}

	child, err := runstate.New(runstate.NewRunRequest{
		Task:          parent.Task,
		Intent:        parent.Intent,
		ParentRunID:   parent.ID,
		RetryOfTaskID: target.ID,
		ProjectID:     parent.ProjectID,
		ProjectRoot:   parent.ProjectRoot,
		Capabilities:  parent.Capabilities,
		Runtime:       parent.Runtime,
		WorkUnits:     []execution.WorkUnit{unit},
	}, time.Now().UTC())
	if err != nil {
		return err
	}
	task := scheduler.Task{
		ID:       target.ID,
		AgentID:  target.AgentID,
		Mutating:       false,
		ToolOperations: execution.CloneToolOperations(target.ToolOperations),
		Status:         scheduler.StatusQueued,
	}
	if err := runstate.BindTaskGraph(&child, []scheduler.Task{task}); err != nil {
		return err
	}
	if err := runStore.Save(child); err != nil {
		return err
	}
	if err := runstate.Transition(&child, runstate.StatusRunning, time.Now().UTC()); err != nil {
		return err
	}
	if err := runStore.Save(child); err != nil {
		return err
	}

	controlStore, err := openRunControlStore(options.controlDir)
	if err != nil {
		return err
	}
	active, err := startActiveRunContext(context.Background(), controlStore, child.ID)
	if err != nil {
		return err
	}
	defer active.Stop()
	ctx := active.Context()

	graph, err := scheduler.New([]scheduler.Task{task})
	if err != nil {
		return err
	}
	inner := coordinator.ExecutionWorker{
		Executor: executor,
		Units: map[string]execution.WorkUnit{
			unit.ID: unit,
		},
		ContextProvider: contextProvider,
	}
	worker := collaboration.HandoffWorker{
		Inner:         inner,
		RunID:         child.ID,
		ArtifactStore: artifactStore,
		MessageBus:    messageBus,
		Dependents:    map[string][]scheduler.Task{},
	}
	sink := &controlplane.RunCoordinatorSink{
		RunID: child.ID,
		Store: eventStore,
		Tasks: []scheduler.Task{task},
		RunData: map[string]any{
			"parent_run_id":    parent.ID,
			"retry_of_task_id": target.ID,
			"retry":            true,
		},
	}

	result, runErr := (coordinator.Coordinator{
		Config: coordinator.Config{MaxConcurrency: 1},
		Workers: map[string]coordinator.Worker{
			target.AgentID: worker,
		},
		Events: sink,
	}).Run(ctx, graph)

	syncRunWorkUnits(&child, graph.Snapshot())
	finalStatus := runstate.StatusCompleted
	if result.Cancelled {
		finalStatus = runstate.StatusCancelled
	} else if runErr != nil || result.Progress.Failed > 0 || result.Progress.Blocked > 0 {
		finalStatus = runstate.StatusFailed
	}
	if err := runstate.Transition(&child, finalStatus, time.Now().UTC()); err != nil {
		return err
	}
	if err := runStore.Save(child); err != nil {
		return err
	}

	if options.json {
		if err := printJSON(struct {
			ParentRunID string             `json:"parent_run_id"`
			RetryTaskID string             `json:"retry_task_id"`
			ChildRun    runstate.Run       `json:"child_run"`
			Result      coordinator.Result `json:"result"`
		}{
			ParentRunID: parent.ID,
			RetryTaskID: target.ID,
			ChildRun:    child,
			Result:      result,
		}); err != nil {
			return err
		}
	} else {
		fmt.Println("Targeted Task Retry")
		fmt.Printf("Parent run: %s\n", parent.ID)
		fmt.Printf("Task: %s (%s)\n", target.ID, target.AgentID)
		fmt.Printf("Child run: %s\n", child.ID)
		fmt.Printf("Status: %s\n", child.Status)
		fmt.Println("Parent run was not modified.")
	}

	if runErr != nil {
		return fmt.Errorf("retry child run %s: %w", child.ID, runErr)
	}
	if finalStatus == runstate.StatusFailed {
		return fmt.Errorf("retry child run %s failed", child.ID)
	}
	return nil
}

func retryTarget(parent runstate.Run, taskID string) (runstate.WorkUnit, error) {
	taskID = strings.TrimSpace(taskID)
	for _, unit := range parent.WorkUnits {
		if unit.ID != taskID {
			continue
		}
		if unit.Mutating {
			return runstate.WorkUnit{}, fmt.Errorf("task %s is mutating and cannot be retried in isolation", taskID)
		}
		switch unit.Status {
		case runstate.WorkUnitFailed, runstate.WorkUnitBlocked:
			return unit, nil
		default:
			return runstate.WorkUnit{}, fmt.Errorf(
				"task %s is not failed or blocked: status=%s",
				taskID,
				unit.Status,
			)
		}
	}
	return runstate.WorkUnit{}, fmt.Errorf("task %s not found in run %s", taskID, parent.ID)
}

func validateRetryDependencies(parent runstate.Run, target runstate.WorkUnit) error {
	statusByID := map[string]runstate.WorkUnitStatus{}
	for _, unit := range parent.WorkUnits {
		statusByID[unit.ID] = unit.Status
	}
	for _, dependency := range target.DependsOn {
		status, ok := statusByID[dependency]
		if !ok {
			return fmt.Errorf("task %s depends on missing durable task %s", target.ID, dependency)
		}
		if status != runstate.WorkUnitCompleted {
			return fmt.Errorf(
				"task %s dependency %s is %s; retry the dependency first",
				target.ID,
				dependency,
				status,
			)
		}
	}
	return nil
}

func retryExecutionUnit(parent runstate.Run, target runstate.WorkUnit) execution.WorkUnit {
	return execution.WorkUnit{
		ID:           target.ID,
		Task:         target.Task,
		ProjectRoot:  parent.ProjectRoot,
		Intent:       orchestration.Intent(parent.Intent),
		AgentID:      target.AgentID,
		Capabilities: append([]string(nil), target.Capabilities...),
		Skills:         append([]string(nil), target.Skills...),
		Tools:          append([]string(nil), target.Tools...),
		ToolOperations: execution.CloneToolOperations(target.ToolOperations),
		Permissions:    target.Permissions,
		Mode:         execution.ModeReadOnly,
	}
}

func parseRunRetryArgs(args []string) (runRetryOptions, error) {
	var options runRetryOptions
	var positional []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--state-dir":
			if i+1 >= len(args) {
				return runRetryOptions{}, fmt.Errorf("--state-dir requiere una ruta")
			}
			i++
			options.stateDir = args[i]
		case "--events-dir":
			if i+1 >= len(args) {
				return runRetryOptions{}, fmt.Errorf("--events-dir requiere una ruta")
			}
			i++
			options.eventsDir = args[i]
		case "--artifacts-dir":
			if i+1 >= len(args) {
				return runRetryOptions{}, fmt.Errorf("--artifacts-dir requiere una ruta")
			}
			i++
			options.artifactsDir = args[i]
		case "--messages-dir":
			if i+1 >= len(args) {
				return runRetryOptions{}, fmt.Errorf("--messages-dir requiere una ruta")
			}
			i++
			options.messagesDir = args[i]
		case "--control-dir":
			if i+1 >= len(args) {
				return runRetryOptions{}, fmt.Errorf("--control-dir requiere una ruta")
			}
			i++
			options.controlDir = args[i]
		case "--json":
			options.json = true
		case "-h", "--help":
			return runRetryOptions{}, fmt.Errorf("uso: ai run retry <run-id> <task-id> [--state-dir ruta] [--events-dir ruta] [--artifacts-dir ruta] [--messages-dir ruta] [--control-dir ruta] [--json]")
		default:
			if strings.HasPrefix(args[i], "-") {
				return runRetryOptions{}, fmt.Errorf("opción desconocida: %s", args[i])
			}
			positional = append(positional, args[i])
		}
	}
	if len(positional) != 2 {
		return runRetryOptions{}, fmt.Errorf("uso: ai run retry <run-id> <task-id>")
	}
	options.runID = positional[0]
	options.taskID = positional[1]
	return options, nil
}
