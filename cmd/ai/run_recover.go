package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/controlplane"
	"github.com/FernandoVillegas27/ai-engineering-os/core/runcheckpoint"
	"github.com/FernandoVillegas27/ai-engineering-os/core/runcontrol"
	"github.com/FernandoVillegas27/ai-engineering-os/core/runstate"
)

type runRecoverOptions struct {
	runID         string
	stateDir      string
	eventsDir     string
	controlDir    string
	checkpointDir string
	staleAfter    time.Duration
	json          bool
}

func runRunRecover(args []string) error {
	options, err := parseRunRecoverArgs(args)
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

	eventStore, err := openControlPlaneStore(options.eventsDir)
	if err != nil {
		return err
	}
	events, err := eventStore.List(run.ID)
	if err != nil {
		return err
	}
	if len(events) == 0 || events[0].Type != controlplane.EventRunStarted {
		return fmt.Errorf("run %s has no valid Control Plane event stream", run.ID)
	}
	state, err := controlplane.Project(events)
	if err != nil {
		return fmt.Errorf("project stale run: %w", err)
	}
	if state.Status != controlplane.RunRunning {
		return fmt.Errorf("Control Plane run %s is not running: status=%s", run.ID, state.Status)
	}

	checkpointAvailable := false
	if hasMutatingWork(run) {
		phase := runcheckpoint.Phase(state.Pipeline.CheckpointPhase)
		switch phase {
		case runcheckpoint.PhaseSourceIntegrated, runcheckpoint.PhaseFinalIntegrated:
		default:
			return fmt.Errorf(
				"mutating run %s has no recoverable checkpoint phase: %q",
				run.ID,
				state.Pipeline.CheckpointPhase,
			)
		}
		checkpointRoot, err := resolveRunAuxDir(
			options.checkpointDir,
			"AI_ENGINEERING_OS_CHECKPOINT_DIR",
			"run-checkpoints",
		)
		if err != nil {
			return err
		}
		checkpointStore, err := runcheckpoint.NewStore(runcheckpoint.Config{Root: checkpointRoot})
		if err != nil {
			return err
		}
		if err := checkpointStore.Check(context.Background(), run.ID, phase); err != nil {
			return fmt.Errorf("mutation recovery checkpoint is invalid: %w", err)
		}
		checkpointAvailable = true
	}

	plan, err := runcontrol.DeriveRecovery(runcontrol.RecoveryInput{
		Run:                 run,
		ControlPlane:        state,
		CheckpointAvailable: checkpointAvailable,
	})
	if err != nil {
		return err
	}

	controlStore, err := openRunControlStore(options.controlDir)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	stale, lease, err := controlStore.LeaseStale(run.ID, now, options.staleAfter)
	if err != nil {
		return fmt.Errorf("inspect run lease: %w", err)
	}
	if !stale {
		return fmt.Errorf(
			"run %s lease is still live; last heartbeat %s",
			run.ID,
			lease.HeartbeatAt.Format(time.RFC3339),
		)
	}

	recoveryLease, previousLease, err := controlStore.ClaimStaleLease(
		run.ID,
		now,
		options.staleAfter,
	)
	if err != nil {
		return fmt.Errorf("claim stale run lease: %w", err)
	}
	releaseRecoveryLease := true
	defer func() {
		if releaseRecoveryLease {
			_ = controlStore.ReleaseLease(run.ID, recoveryLease.OwnerID)
		}
	}()

	original := cloneRunForRecovery(run)
	applyRecoveryPlan(&run, plan)
	transitionAt := time.Now().UTC()
	if transitionAt.Before(run.UpdatedAt) {
		transitionAt = run.UpdatedAt
	}
	if err := runstate.Transition(&run, plan.Status, transitionAt); err != nil {
		return err
	}
	if err := runStore.Save(run); err != nil {
		return err
	}

	event, err := controlplane.NewEvent(controlplane.NewEventRequest{
		RunID:   run.ID,
		Type:    controlplane.EventRunRecovered,
		Summary: "stale active run recovered from durable state",
		Data: map[string]any{
			"status":              string(plan.Status),
			"checkpoint_phase":    plan.CheckpointPhase,
			"tasks":               recoveryTaskEventStatuses(plan),
			"stale_for_ms":        now.Sub(previousLease.HeartbeatAt).Milliseconds(),
			"previous_heartbeat":  previousLease.HeartbeatAt,
		},
	}, time.Now().UTC())
	if err != nil {
		_ = runStore.Save(original)
		return err
	}
	if _, err := eventStore.Append(context.Background(), event); err != nil {
		_ = runStore.Save(original)
		return fmt.Errorf("append recovery event: %w", err)
	}

	if err := controlStore.ReleaseLease(run.ID, recoveryLease.OwnerID); err != nil {
		return fmt.Errorf("release recovery lease: %w", err)
	}
	releaseRecoveryLease = false

	if options.json {
		return printJSON(struct {
			RunID           string                     `json:"run_id"`
			Status          runstate.Status            `json:"status"`
			CheckpointPhase string                     `json:"checkpoint_phase,omitempty"`
			TaskStatuses    map[string]runstate.WorkUnitStatus `json:"task_statuses"`
			StaleFor        string                     `json:"stale_for"`
		}{
			RunID:           run.ID,
			Status:          run.Status,
			CheckpointPhase: plan.CheckpointPhase,
			TaskStatuses:    plan.TaskStatuses,
			StaleFor:        now.Sub(previousLease.HeartbeatAt).String(),
		})
	}

	fmt.Println("Stale Run Recovered")
	fmt.Printf("Run ID: %s\n", run.ID)
	fmt.Printf("Status: %s\n", run.Status)
	if plan.CheckpointPhase != "" {
		fmt.Printf("Checkpoint: %s\n", plan.CheckpointPhase)
	}
	fmt.Printf("Previous lease stale for: %s\n", now.Sub(previousLease.HeartbeatAt).Round(time.Second))
	switch run.Status {
	case runstate.StatusPaused:
		fmt.Printf("Resume: ai run resume %s\n", run.ID)
	case runstate.StatusFailed:
		fmt.Printf("Inspect retryable tasks: ai run control %s\n", run.ID)
	}
	return nil
}

func applyRecoveryPlan(run *runstate.Run, plan runcontrol.RecoveryPlan) {
	if run == nil {
		return
	}
	for index := range run.WorkUnits {
		if status, ok := plan.TaskStatuses[run.WorkUnits[index].ID]; ok {
			run.WorkUnits[index].Status = status
		}
	}
}

func recoveryTaskEventStatuses(plan runcontrol.RecoveryPlan) map[string]string {
	out := make(map[string]string, len(plan.TaskStatuses))
	for taskID, status := range plan.TaskStatuses {
		out[taskID] = string(status)
	}
	return out
}

func cloneRunForRecovery(run runstate.Run) runstate.Run {
	clone := run
	clone.Capabilities = append([]string(nil), run.Capabilities...)
	clone.Approvals = append([]runstate.Approval(nil), run.Approvals...)
	clone.WorkUnits = make([]runstate.WorkUnit, len(run.WorkUnits))
	for i, unit := range run.WorkUnits {
		clone.WorkUnits[i] = unit
		clone.WorkUnits[i].Capabilities = append([]string(nil), unit.Capabilities...)
		clone.WorkUnits[i].Skills = append([]string(nil), unit.Skills...)
		clone.WorkUnits[i].Tools = append([]string(nil), unit.Tools...)
		clone.WorkUnits[i].Scopes = append([]string(nil), unit.Scopes...)
		clone.WorkUnits[i].DependsOn = append([]string(nil), unit.DependsOn...)
	}
	return clone
}

func hasMutatingWork(run runstate.Run) bool {
	for _, unit := range run.WorkUnits {
		if unit.Mutating {
			return true
		}
	}
	return false
}

func parseRunRecoverArgs(args []string) (runRecoverOptions, error) {
	options := runRecoverOptions{
		staleAfter: 30 * time.Second,
	}
	var positional []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--state-dir":
			if i+1 >= len(args) {
				return runRecoverOptions{}, fmt.Errorf("--state-dir requiere una ruta")
			}
			i++
			options.stateDir = args[i]
		case "--events-dir":
			if i+1 >= len(args) {
				return runRecoverOptions{}, fmt.Errorf("--events-dir requiere una ruta")
			}
			i++
			options.eventsDir = args[i]
		case "--control-dir":
			if i+1 >= len(args) {
				return runRecoverOptions{}, fmt.Errorf("--control-dir requiere una ruta")
			}
			i++
			options.controlDir = args[i]
		case "--checkpoint-dir":
			if i+1 >= len(args) {
				return runRecoverOptions{}, fmt.Errorf("--checkpoint-dir requiere una ruta")
			}
			i++
			options.checkpointDir = args[i]
		case "--stale-after":
			if i+1 >= len(args) {
				return runRecoverOptions{}, fmt.Errorf("--stale-after requiere una duración")
			}
			i++
			value, err := time.ParseDuration(args[i])
			if err != nil {
				return runRecoverOptions{}, fmt.Errorf("--stale-after inválido: %w", err)
			}
			if value < 5*time.Second {
				return runRecoverOptions{}, fmt.Errorf("--stale-after debe ser al menos 5s")
			}
			options.staleAfter = value
		case "--json":
			options.json = true
		case "-h", "--help":
			return runRecoverOptions{}, fmt.Errorf("uso: ai run recover <run-id> [--stale-after 30s] [--state-dir ruta] [--events-dir ruta] [--control-dir ruta] [--checkpoint-dir ruta] [--json]")
		default:
			if strings.HasPrefix(args[i], "-") {
				return runRecoverOptions{}, fmt.Errorf("opción desconocida: %s", args[i])
			}
			positional = append(positional, args[i])
		}
	}
	if len(positional) != 1 {
		return runRecoverOptions{}, fmt.Errorf("uso: ai run recover <run-id>")
	}
	options.runID = positional[0]
	return options, nil
}
