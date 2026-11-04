package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/controlplane"
	"github.com/FernandoVillegas27/ai-engineering-os/core/runcontrol"
	"github.com/FernandoVillegas27/ai-engineering-os/core/runstate"
)

type runCancelOptions struct {
	runID      string
	controlDir string
	stateDir   string
	eventsDir  string
	json       bool
}

func runRunCancel(args []string) error {
	options, err := parseRunCancelArgs(args)
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
	switch run.Status {
	case runstate.StatusCompleted, runstate.StatusFailed, runstate.StatusCancelled:
		return fmt.Errorf("run %s is already terminal with status %s", run.ID, run.Status)
	}

	controlStore, err := openRunControlStore(options.controlDir)
	if err != nil {
		return err
	}
	request, err := controlStore.RequestCancel(run.ID, time.Now().UTC())
	if err != nil {
		return err
	}

	eventStore, err := openControlPlaneStore(options.eventsDir)
	if err != nil {
		return err
	}
	event, err := controlplane.NewEvent(controlplane.NewEventRequest{
		RunID:   run.ID,
		Type:    controlplane.EventRunCancelRequested,
		Summary: "run cancellation requested",
		Data: map[string]any{
			"requested_at": request.RequestedAt,
		},
	}, request.RequestedAt)
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
	alreadyRecorded := false
	for _, existing := range events {
		if existing.Type == controlplane.EventRunCancelRequested {
			alreadyRecorded = true
			break
		}
	}
	if !alreadyRecorded {
		if _, err := eventStore.Append(context.Background(), event); err != nil {
			return err
		}
	}

	if options.json {
		return printJSON(request)
	}
	fmt.Println("Run Cancellation Requested")
	fmt.Printf("Run ID: %s\n", run.ID)
	fmt.Printf("Status: %s\n", run.Status)
	fmt.Printf("Requested: %s\n", request.RequestedAt.Format(time.RFC3339))
	fmt.Println("The active runner will stop at the cancellation boundary.")
	return nil
}

func parseRunCancelArgs(args []string) (runCancelOptions, error) {
	var options runCancelOptions
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--control-dir":
			if i+1 >= len(args) {
				return runCancelOptions{}, fmt.Errorf("--control-dir requiere una ruta")
			}
			i++
			options.controlDir = args[i]
		case "--state-dir":
			if i+1 >= len(args) {
				return runCancelOptions{}, fmt.Errorf("--state-dir requiere una ruta")
			}
			i++
			options.stateDir = args[i]
		case "--events-dir":
			if i+1 >= len(args) {
				return runCancelOptions{}, fmt.Errorf("--events-dir requiere una ruta")
			}
			i++
			options.eventsDir = args[i]
		case "--json":
			options.json = true
		case "-h", "--help":
			return runCancelOptions{}, fmt.Errorf("uso: ai run cancel <run-id> [--control-dir ruta] [--state-dir ruta] [--events-dir ruta] [--json]")
		default:
			if strings.HasPrefix(args[i], "-") {
				return runCancelOptions{}, fmt.Errorf("opción desconocida: %s", args[i])
			}
			if options.runID != "" {
				return runCancelOptions{}, fmt.Errorf("se esperaba un solo run id")
			}
			options.runID = args[i]
		}
	}
	if strings.TrimSpace(options.runID) == "" {
		return runCancelOptions{}, fmt.Errorf("run id es obligatorio")
	}
	return options, nil
}

func openRunControlStore(explicit string) (*runcontrol.Store, error) {
	root, err := resolveRunAuxDir(explicit, "AI_ENGINEERING_OS_CONTROL_DIR", "run-control")
	if err != nil {
		return nil, err
	}
	return runcontrol.NewStore(root)
}
