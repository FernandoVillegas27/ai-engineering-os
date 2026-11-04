package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/classifier"
	"github.com/FernandoVillegas27/ai-engineering-os/core/controlplane"
	"github.com/FernandoVillegas27/ai-engineering-os/core/coordinator"
	"github.com/FernandoVillegas27/ai-engineering-os/core/detector"
	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/orchestration"
	"github.com/FernandoVillegas27/ai-engineering-os/core/registry"
	"github.com/FernandoVillegas27/ai-engineering-os/core/runstate"
	"github.com/FernandoVillegas27/ai-engineering-os/core/scheduler"
	"github.com/FernandoVillegas27/ai-engineering-os/core/taskgraph"
	codexintegration "github.com/FernandoVillegas27/ai-engineering-os/integrations/codex"
)

type runExecuteOptions struct {
	root           string
	task           string
	projectID      string
	useAI          bool
	dryRun         bool
	runtime        string
	stateDir       string
	eventsDir      string
	maxConcurrency int
	json           bool
}

func runRunExecute(args []string) error {
	options, err := parseRunExecuteArgs(args)
	if err != nil {
		return err
	}

	root, err := filepath.Abs(options.root)
	if err != nil {
		return fmt.Errorf("resolver ruta del proyecto: %w", err)
	}
	if options.projectID == "" {
		options.projectID = filepath.Base(root)
	}

	capabilitiesPath, err := resolveRegistryFile("capabilities.yaml")
	if err != nil {
		return err
	}
	agentsPath, err := resolveRegistryFile("agents.yaml")
	if err != nil {
		return err
	}
	skillsPath, err := resolveRegistryFile("skills.yaml")
	if err != nil {
		return err
	}
	toolsPath, err := resolveRegistryFile("tools.yaml")
	if err != nil {
		return err
	}

	capabilities, err := registry.LoadCapabilities(capabilitiesPath)
	if err != nil {
		return err
	}
	agents, err := registry.LoadAgents(agentsPath)
	if err != nil {
		return err
	}
	skills, err := registry.LoadSkills(skillsPath)
	if err != nil {
		return err
	}
	tools, err := registry.LoadTools(toolsPath)
	if err != nil {
		return err
	}
	detection, err := detector.Inspect(root)
	if err != nil {
		return fmt.Errorf("inspeccionar proyecto: %w", err)
	}

	classified, err := classifyRunTask(context.Background(), options, capabilities, root)
	if err != nil {
		return err
	}

	plan, err := orchestration.Build(
		orchestration.Request{
			Capabilities: classified.Capabilities,
			Intent:       classified.Intent,
		},
		detection,
		agents,
		skills,
		tools,
	)
	if err != nil {
		return err
	}

	units, err := execution.BuildWorkUnits(options.task, root, plan, agents)
	if err != nil {
		return err
	}

	runStore, err := openRunStore(options.stateDir)
	if err != nil {
		return err
	}
	eventStore, err := openControlPlaneStore(options.eventsDir)
	if err != nil {
		return err
	}

	runtimeSelection := runstate.RuntimeSelection{
		Provider: options.runtime,
	}
	if options.dryRun {
		runtimeSelection.Provider = "dry-run"
	}
	run, err := runstate.New(runstate.NewRunRequest{
		Task:         options.task,
		ProjectID:    options.projectID,
		ProjectRoot:  root,
		Capabilities: plan.RequestedCapabilities,
		Runtime:      runtimeSelection,
		WorkUnits:    units,
	}, time.Now().UTC())
	if err != nil {
		return err
	}
	if err := runStore.Save(run); err != nil {
		return err
	}

	tasks, err := taskgraph.Build(units)
	if err != nil {
		return err
	}
	unitByID := make(map[string]execution.WorkUnit, len(units))
	for _, unit := range units {
		unitByID[unit.ID] = unit
	}
	graph, err := scheduler.New(tasks)
	if err != nil {
		return err
	}

	workers := make(map[string]coordinator.Worker, len(units))
	for _, unit := range units {
		var executor execution.Executor
		if options.dryRun {
			executor = execution.DryRunExecutor{}
		} else {
			executor = execution.RuntimeExecutor{Runtime: codexintegration.NewExecutionRuntime()}
		}
		workers[unit.AgentID] = coordinator.ExecutionWorker{
			Executor: executor,
			Units:    unitByID,
		}
	}

	sink := &controlplane.RunCoordinatorSink{
		RunID: run.ID,
		Store: eventStore,
		Tasks: tasks,
	}
	if err := runstate.Transition(&run, runstate.StatusRunning, time.Now().UTC()); err != nil {
		return err
	}
	if err := runStore.Save(run); err != nil {
		return err
	}

	ctx := context.Background()
	result, runErr := (coordinator.Coordinator{
		Config: coordinator.Config{MaxConcurrency: options.maxConcurrency},
		Workers: workers,
		Events:  sink,
	}).Run(ctx, graph)

	syncRunWorkUnits(&run, graph.Snapshot())
	finalStatus := runstate.StatusCompleted
	if result.Cancelled {
		finalStatus = runstate.StatusCancelled
	} else if runErr != nil || result.Progress.Failed > 0 || result.Progress.Blocked > 0 || result.Progress.Cancelled > 0 {
		finalStatus = runstate.StatusFailed
	}
	if transitionErr := runstate.Transition(&run, finalStatus, time.Now().UTC()); transitionErr != nil {
		return transitionErr
	}
	if saveErr := runStore.Save(run); saveErr != nil {
		return saveErr
	}

	if options.json {
		payload := struct {
			Run    runstate.Run      `json:"run"`
			Result coordinator.Result `json:"result"`
		}{Run: run, Result: result}
		if err := printJSON(payload); err != nil {
			return err
		}
	} else {
		fmt.Println("Coordinated Run")
		fmt.Printf("Run ID: %s\n", run.ID)
		fmt.Printf("Project: %s\n", run.ProjectID)
		fmt.Printf("Intent: %s\n", plan.Intent)
		fmt.Printf("Agents: %d\n", len(units))
		fmt.Printf("Concurrency: %d\n", options.maxConcurrency)
		fmt.Printf("Status: %s\n", run.Status)
		fmt.Printf(
			"Progress: completed=%d failed=%d blocked=%d cancelled=%d total=%d\n",
			result.Progress.Completed,
			result.Progress.Failed,
			result.Progress.Blocked,
			result.Progress.Cancelled,
			result.Progress.Total,
		)
		fmt.Println()
		fmt.Printf("Inspect: ai run control %s\n", run.ID)
		fmt.Printf("Events:  ai run events %s\n", run.ID)
	}

	if runErr != nil {
		return runErr
	}
	if finalStatus != runstate.StatusCompleted {
		return fmt.Errorf("coordinated run finished with status %s", finalStatus)
	}
	return nil
}

func classifyRunTask(
	ctx context.Context,
	options runExecuteOptions,
	capabilities registry.CapabilityRegistry,
	root string,
) (classifier.Result, error) {
	if options.useAI {
		resolution, err := classifier.Resolve(
			ctx,
			options.task,
			capabilities,
			readNormalizedProjectContext(root),
			codexintegration.New(),
		)
		if err != nil {
			return classifier.Result{}, err
		}
		if resolution.ClarificationRequired {
			return classifier.Result{}, fmt.Errorf("se requiere aclaración antes de ejecutar: %s", resolution.ClarificationQuestion)
		}
		if resolution.Result.NeedsAI {
			return classifier.Result{}, fmt.Errorf("la tarea sigue ambigua y no pudo clasificarse")
		}
		return resolution.Result, nil
	}

	classified, err := classifier.Classify(options.task, capabilities)
	if err != nil {
		return classifier.Result{}, err
	}
	if classified.NeedsAI {
		return classifier.Result{}, fmt.Errorf("la tarea es ambigua; repetí con --ai para habilitar el fallback de Codex")
	}
	return classified, nil
}

func syncRunWorkUnits(run *runstate.Run, tasks []scheduler.Task) {
	if run == nil {
		return
	}
	statusByID := map[string]runstate.WorkUnitStatus{}
	for _, task := range tasks {
		statusByID[task.ID] = schedulerToRunState(task.Status)
	}
	for index := range run.WorkUnits {
		if status, ok := statusByID[run.WorkUnits[index].ID]; ok {
			run.WorkUnits[index].Status = status
		}
	}
}

func schedulerToRunState(status scheduler.Status) runstate.WorkUnitStatus {
	switch status {
	case scheduler.StatusQueued:
		return runstate.WorkUnitQueued
	case scheduler.StatusReady:
		return runstate.WorkUnitReady
	case scheduler.StatusRunning:
		return runstate.WorkUnitRunning
	case scheduler.StatusWaiting:
		return runstate.WorkUnitWaiting
	case scheduler.StatusBlocked:
		return runstate.WorkUnitBlocked
	case scheduler.StatusCompleted:
		return runstate.WorkUnitCompleted
	case scheduler.StatusFailed:
		return runstate.WorkUnitFailed
	case scheduler.StatusCancelled:
		return runstate.WorkUnitCancelled
	default:
		return runstate.WorkUnitQueued
	}
}

func parseRunExecuteArgs(args []string) (runExecuteOptions, error) {
	options := runExecuteOptions{
		root:           ".",
		maxConcurrency: 2,
	}
	var positional []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--task":
			if i+1 >= len(args) {
				return runExecuteOptions{}, fmt.Errorf("--task requiere texto")
			}
			i++
			options.task = args[i]
		case "--project":
			if i+1 >= len(args) {
				return runExecuteOptions{}, fmt.Errorf("--project requiere un id")
			}
			i++
			options.projectID = args[i]
		case "--ai":
			options.useAI = true
		case "--dry-run":
			options.dryRun = true
		case "--runtime":
			if i+1 >= len(args) {
				return runExecuteOptions{}, fmt.Errorf("--runtime requiere un valor")
			}
			i++
			options.runtime = args[i]
		case "--state-dir":
			if i+1 >= len(args) {
				return runExecuteOptions{}, fmt.Errorf("--state-dir requiere una ruta")
			}
			i++
			options.stateDir = args[i]
		case "--events-dir":
			if i+1 >= len(args) {
				return runExecuteOptions{}, fmt.Errorf("--events-dir requiere una ruta")
			}
			i++
			options.eventsDir = args[i]
		case "--max-concurrency":
			if i+1 >= len(args) {
				return runExecuteOptions{}, fmt.Errorf("--max-concurrency requiere un entero")
			}
			i++
			value, err := strconv.Atoi(args[i])
			if err != nil || value < 1 || value > 16 {
				return runExecuteOptions{}, fmt.Errorf("--max-concurrency debe estar entre 1 y 16")
			}
			options.maxConcurrency = value
		case "--json":
			options.json = true
		case "-h", "--help":
			return runExecuteOptions{}, fmt.Errorf("uso: ai run execute [ruta] --task <tarea> (--dry-run | --runtime codex) [--project id] [--max-concurrency N] [--ai]")
		default:
			if strings.HasPrefix(args[i], "-") {
				return runExecuteOptions{}, fmt.Errorf("opción desconocida: %s", args[i])
			}
			positional = append(positional, args[i])
		}
	}
	if len(positional) > 1 {
		return runExecuteOptions{}, fmt.Errorf("se esperaba una sola ruta de proyecto")
	}
	if len(positional) == 1 {
		options.root = positional[0]
	}
	if strings.TrimSpace(options.task) == "" {
		return runExecuteOptions{}, fmt.Errorf("--task es obligatorio")
	}
	if options.dryRun && options.runtime != "" {
		return runExecuteOptions{}, fmt.Errorf("--dry-run y --runtime no se pueden usar juntos")
	}
	if !options.dryRun && options.runtime == "" {
		return runExecuteOptions{}, fmt.Errorf("indicá --dry-run o --runtime codex")
	}
	if options.runtime != "" && options.runtime != "codex" {
		return runExecuteOptions{}, fmt.Errorf("runtime no soportado: %s", options.runtime)
	}
	return options, nil
}
