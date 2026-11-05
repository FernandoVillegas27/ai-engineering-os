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
	"github.com/FernandoVillegas27/ai-engineering-os/core/runcontrol"
	"github.com/FernandoVillegas27/ai-engineering-os/core/runstate"
	"github.com/FernandoVillegas27/ai-engineering-os/core/scheduler"
)

type runResumeOptions struct {
	runID          string
	stateDir       string
	eventsDir      string
	artifactsDir   string
	messagesDir    string
	controlDir     string
	maxConcurrency int
	json           bool
}

func runRunResume(args []string) error {
	options, err := parseRunResumeArgs(args)
	if err != nil {
		return err
	}

	runStore, err := openRunStore(options.stateDir)
	if err != nil {
		return err
	}
	run, err := runStore.Load(options.runID)
	if err != nil {
		return fmt.Errorf("load run: %w", err)
	}
	if run.Status != runstate.StatusPaused {
		return fmt.Errorf("run %s is not paused: status=%s", run.ID, run.Status)
	}
	if strings.TrimSpace(run.Intent) == "" {
		return fmt.Errorf("run %s has no durable intent and cannot resume safely", run.ID)
	}
	for _, unit := range run.WorkUnits {
		if unit.Mutating {
			return fmt.Errorf("run %s contains mutating work; durable resume is currently read-only only", run.ID)
		}
	}

	eventStore, err := openControlPlaneStore(options.eventsDir)
	if err != nil {
		return err
	}
	events, err := eventStore.List(run.ID)
	if err != nil {
		return err
	}
	if len(events) == 0 || events[0].Type != controlplane.EventRunStarted {
		return fmt.Errorf("run %s has no valid Control Plane event stream in the selected --events-dir", run.ID)
	}
	state, err := controlplane.Project(events)
	if err != nil {
		return fmt.Errorf("project paused run state: %w", err)
	}
	if state.Status != controlplane.RunPaused {
		return fmt.Errorf("Control Plane run %s is not paused: status=%s", run.ID, state.Status)
	}

	units, tasks, err := reconstructPausedReadOnlyRun(run)
	if err != nil {
		return err
	}
	graph, err := scheduler.New(tasks)
	if err != nil {
		return fmt.Errorf("reconstruct paused scheduler: %w", err)
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

	unitByID := make(map[string]execution.WorkUnit, len(units))
	for _, unit := range units {
		unitByID[unit.ID] = unit
	}
	dependents := collaboration.BuildDependents(tasks)

	var executor execution.Executor
	switch run.Runtime.Provider {
	case "dry-run":
		executor = execution.DryRunExecutor{}
	case "auto", "codex", "cursor":
		policy := modelpolicy.PolicyAuto
		if strings.TrimSpace(run.Runtime.ModelPolicy) != "" {
			policy, err = modelpolicy.Parse(run.Runtime.ModelPolicy)
			if err != nil {
				return fmt.Errorf("parse durable model policy: %w", err)
			}
		}
		runtime, _, runtimeErr := executionRuntimeFor(
			run.Runtime.Provider,
			modelRoutingOptions{Policy: policy, Override: run.Runtime.Model},
		)
		if runtimeErr != nil {
			return runtimeErr
		}
		executor = execution.RuntimeExecutor{Runtime: runtime}
	default:
		return fmt.Errorf("resume does not support durable runtime %q", run.Runtime.Provider)
	}

	workers := make(map[string]coordinator.Worker, len(units))
	for _, unit := range units {
		inner := coordinator.ExecutionWorker{
			Executor: executor,
			Units:    unitByID,
			ContextProvider: collaboration.InboxContextProvider{
				RunID:         run.ID,
				MessageBus:    messageBus,
				ArtifactStore: artifactStore,
			},
		}
		workers[unit.AgentID] = collaboration.HandoffWorker{
			Inner:         inner,
			RunID:         run.ID,
			ArtifactStore: artifactStore,
			MessageBus:    messageBus,
			Dependents:    dependents,
		}
	}

	controlStore, err := openRunControlStore(options.controlDir)
	if err != nil {
		return err
	}
	requested, err := controlStore.PauseRequested(run.ID)
	if err != nil {
		return err
	}
	if !requested {
		return fmt.Errorf("run %s has no durable pause request to clear", run.ID)
	}
	if err := controlStore.ClearPause(run.ID); err != nil {
		return err
	}

	if err := runstate.Transition(&run, runstate.StatusRunning, time.Now().UTC()); err != nil {
		return err
	}
	if err := runStore.Save(run); err != nil {
		return err
	}

	ctx, stopWatcher, err := runcontrol.WithCancellationWatcher(
		context.Background(),
		controlStore,
		run.ID,
		250*time.Millisecond,
	)
	if err != nil {
		return err
	}
	defer stopWatcher()

	sink := &controlplane.RunCoordinatorSink{
		RunID:  run.ID,
		Store:  eventStore,
		Tasks:  tasks,
		Resume: true,
	}
	result, runErr := (coordinator.Coordinator{
		Config: coordinator.Config{
			MaxConcurrency: options.maxConcurrency,
			Resume:         true,
			PauseRequested: func() (bool, error) {
				return controlStore.PauseRequested(run.ID)
			},
		},
		Workers: workers,
		Events:  sink,
	}).Run(ctx, graph)

	syncRunWorkUnits(&run, graph.Snapshot())
	finalStatus := runstate.StatusCompleted
	if result.Paused {
		finalStatus = runstate.StatusPaused
	} else if result.Cancelled {
		finalStatus = runstate.StatusCancelled
	} else if runErr != nil || result.Progress.Failed > 0 || result.Progress.Blocked > 0 || result.Progress.Cancelled > 0 {
		finalStatus = runstate.StatusFailed
	}
	if err := runstate.Transition(&run, finalStatus, time.Now().UTC()); err != nil {
		return err
	}
	if err := runStore.Save(run); err != nil {
		return err
	}

	if options.json {
		return printJSON(struct {
			Run    runstate.Run       `json:"run"`
			Result coordinator.Result `json:"result"`
		}{Run: run, Result: result})
	}

	fmt.Println("Resumed Coordinated Run")
	fmt.Printf("Run ID: %s\n", run.ID)
	fmt.Printf("Status: %s\n", run.Status)
	fmt.Printf(
		"Progress: completed=%d failed=%d blocked=%d cancelled=%d total=%d\n",
		result.Progress.Completed,
		result.Progress.Failed,
		result.Progress.Blocked,
		result.Progress.Cancelled,
		result.Progress.Total,
	)
	fmt.Printf("Inspect: ai run control %s\n", run.ID)

	if runErr != nil {
		return runErr
	}
	if finalStatus == runstate.StatusPaused {
		return nil
	}
	if finalStatus != runstate.StatusCompleted {
		return fmt.Errorf("resumed run finished with status %s", finalStatus)
	}
	return nil
}

func reconstructPausedReadOnlyRun(run runstate.Run) ([]execution.WorkUnit, []scheduler.Task, error) {
	units := make([]execution.WorkUnit, 0, len(run.WorkUnits))
	tasks := make([]scheduler.Task, 0, len(run.WorkUnits))
	for _, durable := range run.WorkUnits {
		if durable.Mutating {
			return nil, nil, fmt.Errorf("work unit %s is mutating", durable.ID)
		}
		status := scheduler.StatusQueued
		switch durable.Status {
		case runstate.WorkUnitCompleted:
			status = scheduler.StatusCompleted
		case runstate.WorkUnitQueued, runstate.WorkUnitReady, runstate.WorkUnitWaiting:
			status = scheduler.StatusQueued
		case runstate.WorkUnitRunning:
			return nil, nil, fmt.Errorf("paused run contains running work unit %s", durable.ID)
		case runstate.WorkUnitFailed, runstate.WorkUnitBlocked, runstate.WorkUnitCancelled:
			return nil, nil, fmt.Errorf(
				"paused run contains terminal work unit %s with status %s",
				durable.ID,
				durable.Status,
			)
		default:
			return nil, nil, fmt.Errorf("unsupported paused work-unit status for %s: %s", durable.ID, durable.Status)
		}

		units = append(units, execution.WorkUnit{
			ID:           durable.ID,
			Task:         durable.Task,
			ProjectRoot:  run.ProjectRoot,
			Intent:       orchestration.Intent(run.Intent),
			AgentID:      durable.AgentID,
			Capabilities: append([]string(nil), durable.Capabilities...),
			Skills:       append([]string(nil), durable.Skills...),
			Tools:        append([]string(nil), durable.Tools...),
			Permissions:  durable.Permissions,
			Mode:         execution.ModeReadOnly,
		})
		tasks = append(tasks, scheduler.Task{
			ID:        durable.ID,
			AgentID:   durable.AgentID,
			DependsOn: append([]string(nil), durable.DependsOn...),
			Mutating:  false,
			Status:    status,
		})
	}
	return units, tasks, nil
}

func parseRunResumeArgs(args []string) (runResumeOptions, error) {
	options := runResumeOptions{maxConcurrency: 2}
	var positional []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--state-dir":
			if i+1 >= len(args) {
				return runResumeOptions{}, fmt.Errorf("--state-dir requiere una ruta")
			}
			i++
			options.stateDir = args[i]
		case "--events-dir":
			if i+1 >= len(args) {
				return runResumeOptions{}, fmt.Errorf("--events-dir requiere una ruta")
			}
			i++
			options.eventsDir = args[i]
		case "--artifacts-dir":
			if i+1 >= len(args) {
				return runResumeOptions{}, fmt.Errorf("--artifacts-dir requiere una ruta")
			}
			i++
			options.artifactsDir = args[i]
		case "--messages-dir":
			if i+1 >= len(args) {
				return runResumeOptions{}, fmt.Errorf("--messages-dir requiere una ruta")
			}
			i++
			options.messagesDir = args[i]
		case "--control-dir":
			if i+1 >= len(args) {
				return runResumeOptions{}, fmt.Errorf("--control-dir requiere una ruta")
			}
			i++
			options.controlDir = args[i]
		case "--max-concurrency":
			if i+1 >= len(args) {
				return runResumeOptions{}, fmt.Errorf("--max-concurrency requiere un valor")
			}
			i++
			var parsed int
			if _, err := fmt.Sscanf(args[i], "%d", &parsed); err != nil || parsed < 1 || parsed > 16 {
				return runResumeOptions{}, fmt.Errorf("--max-concurrency debe estar entre 1 y 16")
			}
			options.maxConcurrency = parsed
		case "--json":
			options.json = true
		case "-h", "--help":
			return runResumeOptions{}, fmt.Errorf("uso: ai run resume <run-id> [--state-dir ruta] [--events-dir ruta] [--artifacts-dir ruta] [--messages-dir ruta] [--control-dir ruta] [--max-concurrency N] [--json]")
		default:
			if strings.HasPrefix(args[i], "-") {
				return runResumeOptions{}, fmt.Errorf("opción desconocida: %s", args[i])
			}
			positional = append(positional, args[i])
		}
	}
	if len(positional) != 1 {
		return runResumeOptions{}, fmt.Errorf("uso: ai run resume <run-id>")
	}
	options.runID = positional[0]
	return options, nil
}
