package main

import (
	"context"
	"fmt"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/controlplane"
	"github.com/FernandoVillegas27/ai-engineering-os/core/runcheckpoint"
	"github.com/FernandoVillegas27/ai-engineering-os/core/runcontrol"
	"github.com/FernandoVillegas27/ai-engineering-os/core/runstate"
)

func persistMutationCheckpointState(
	run *runstate.Run,
	store *runstate.Store,
	phase runcheckpoint.Phase,
) error {
	if run == nil || store == nil {
		return fmt.Errorf("mutation checkpoint state requires run and store")
	}
	for index := range run.WorkUnits {
		unit := &run.WorkUnits[index]
		switch phase {
		case runcheckpoint.PhaseSourceIntegrated:
			if unit.MutationKind == "tests-only" {
				if unit.Status != runstate.WorkUnitCompleted {
					unit.Status = runstate.WorkUnitQueued
				}
				continue
			}
			unit.Status = runstate.WorkUnitCompleted
		case runcheckpoint.PhaseFinalIntegrated:
			unit.Status = runstate.WorkUnitCompleted
		default:
			return fmt.Errorf("unsupported mutation checkpoint phase: %s", phase)
		}
	}
	now := time.Now().UTC()
	if now.Before(run.UpdatedAt) {
		now = run.UpdatedAt
	}
	run.UpdatedAt = now
	if err := runstate.Validate(*run); err != nil {
		return err
	}
	return store.Save(*run)
}

func pauseMutationAtCheckpoint(
	ctx context.Context,
	run *runstate.Run,
	runStore *runstate.Store,
	eventStore *controlplane.EventStore,
	controlStore *runcontrol.Store,
	phase runcheckpoint.Phase,
) (bool, error) {
	if err := persistMutationCheckpointState(run, runStore, phase); err != nil {
		return false, err
	}
	requested, err := controlStore.PauseRequested(run.ID)
	if err != nil {
		return false, err
	}
	if !requested {
		return false, nil
	}

	now := time.Now().UTC()
	if now.Before(run.UpdatedAt) {
		now = run.UpdatedAt
	}
	if err := runstate.Transition(run, runstate.StatusPaused, now); err != nil {
		return false, err
	}
	if err := runStore.Save(*run); err != nil {
		return false, err
	}
	if err := appendSimpleRunEvent(
		context.WithoutCancel(ctx),
		eventStore,
		run.ID,
		controlplane.EventRunPaused,
		"", "",
		fmt.Sprintf("coordinated mutating run paused at %s checkpoint", phase),
		string(phase),
	); err != nil {
		rollbackAt := time.Now().UTC()
		if rollbackAt.Before(run.UpdatedAt) {
			rollbackAt = run.UpdatedAt
		}
		if transitionErr := runstate.Transition(run, runstate.StatusRunning, rollbackAt); transitionErr == nil {
			_ = runStore.Save(*run)
		}
		_ = controlStore.ClearPause(run.ID)
		return false, err
	}
	return true, nil
}

func renderMutationPaused(options runExecuteOptions, run runstate.Run, phase runcheckpoint.Phase) error {
	if options.json {
		return printJSON(struct {
			RunID string              `json:"run_id"`
			Status runstate.Status     `json:"status"`
			Phase  runcheckpoint.Phase `json:"checkpoint_phase"`
		}{
			RunID: run.ID,
			Status: run.Status,
			Phase:  phase,
		})
	}
	fmt.Println("Coordinated Mutating Run Paused")
	fmt.Printf("Run ID: %s\n", run.ID)
	fmt.Printf("Checkpoint: %s\n", phase)
	fmt.Println("Original project was NOT modified.")
	fmt.Printf("Resume: ai run resume %s\n", run.ID)
	return nil
}
