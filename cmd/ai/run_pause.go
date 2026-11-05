package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/controlplane"
	"github.com/FernandoVillegas27/ai-engineering-os/core/runstate"
)

type runPauseOptions struct {
	runID      string
	controlDir string
	stateDir   string
	eventsDir  string
	json       bool
}

func runRunPause(args []string) error {
	options, err := parseRunPauseArgs(args)
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
	if run.Status != runstate.StatusRunning {
		return fmt.Errorf("run %s is not running: status=%s", run.ID, run.Status)
	}
	mutating := false
	for _, unit := range run.WorkUnits {
		if unit.Mutating {
			mutating = true
			if unit.MutationKind == "" {
				return fmt.Errorf(
					"run %s contains legacy mutating work without mutation_kind; checkpoint pause is unsafe",
					run.ID,
				)
			}
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
		return fmt.Errorf("project run state: %w", err)
	}
	if state.Status != controlplane.RunRunning {
		return fmt.Errorf("Control Plane run %s is not running: status=%s", run.ID, state.Status)
	}

	controlStore, err := openRunControlStore(options.controlDir)
	if err != nil {
		return err
	}
	request, err := controlStore.RequestPause(run.ID, time.Now().UTC())
	if err != nil {
		return err
	}

	alreadyRecorded := false
	for _, existing := range events {
		if existing.Type == controlplane.EventRunPauseRequested && state.Status == controlplane.RunRunning {
			alreadyRecorded = true
			break
		}
	}
	if !alreadyRecorded {
		event, err := controlplane.NewEvent(controlplane.NewEventRequest{
			RunID:   run.ID,
			Type:    controlplane.EventRunPauseRequested,
			Summary: "run pause requested",
			Data: map[string]any{
				"requested_at": request.RequestedAt,
			},
		}, request.RequestedAt)
		if err != nil {
			return err
		}
		if _, err := eventStore.Append(context.Background(), event); err != nil {
			_ = controlStore.ClearPause(run.ID)
			return err
		}
	}

	if options.json {
		return printJSON(request)
	}
	fmt.Println("Run Pause Requested")
	fmt.Printf("Run ID: %s\n", run.ID)
	fmt.Printf("Status: %s\n", run.Status)
	fmt.Printf("Requested: %s\n", request.RequestedAt.Format(time.RFC3339))
	if mutating {
		fmt.Println("The mutating run will pause at the next durable integration checkpoint.")
	} else {
		fmt.Println("No new tasks will start after the active task boundary.")
	}
	return nil
}

func parseRunPauseArgs(args []string) (runPauseOptions, error) {
	var options runPauseOptions
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--control-dir":
			if i+1 >= len(args) {
				return runPauseOptions{}, fmt.Errorf("--control-dir requiere una ruta")
			}
			i++
			options.controlDir = args[i]
		case "--state-dir":
			if i+1 >= len(args) {
				return runPauseOptions{}, fmt.Errorf("--state-dir requiere una ruta")
			}
			i++
			options.stateDir = args[i]
		case "--events-dir":
			if i+1 >= len(args) {
				return runPauseOptions{}, fmt.Errorf("--events-dir requiere una ruta")
			}
			i++
			options.eventsDir = args[i]
		case "--json":
			options.json = true
		case "-h", "--help":
			return runPauseOptions{}, fmt.Errorf("uso: ai run pause <run-id> [--control-dir ruta] [--state-dir ruta] [--events-dir ruta] [--json]")
		default:
			if strings.HasPrefix(args[i], "-") {
				return runPauseOptions{}, fmt.Errorf("opción desconocida: %s", args[i])
			}
			if options.runID != "" {
				return runPauseOptions{}, fmt.Errorf("se esperaba un solo run id")
			}
			options.runID = args[i]
		}
	}
	if strings.TrimSpace(options.runID) == "" {
		return runPauseOptions{}, fmt.Errorf("run id es obligatorio")
	}
	return options, nil
}
