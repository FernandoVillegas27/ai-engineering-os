package runcontrol

import (
	"errors"
	"fmt"

	"github.com/FernandoVillegas27/ai-engineering-os/core/controlplane"
	"github.com/FernandoVillegas27/ai-engineering-os/core/runcheckpoint"
	"github.com/FernandoVillegas27/ai-engineering-os/core/runstate"
)

type RecoveryPlan struct {
	Status          runstate.Status
	CheckpointPhase string
	TaskStatuses    map[string]runstate.WorkUnitStatus
}

type RecoveryInput struct {
	Run                 runstate.Run
	ControlPlane        controlplane.State
	CheckpointAvailable bool
}

func DeriveRecovery(input RecoveryInput) (RecoveryPlan, error) {
	run := input.Run
	state := input.ControlPlane
	if run.ID == "" || state.RunID == "" || run.ID != state.RunID {
		return RecoveryPlan{}, errors.New("runstate/control-plane identity mismatch")
	}
	if run.Status != runstate.StatusRunning || state.Status != controlplane.RunRunning {
		return RecoveryPlan{}, fmt.Errorf(
			"recovery requires running state: runstate=%s control-plane=%s",
			run.Status,
			state.Status,
		)
	}
	if len(run.WorkUnits) == 0 {
		return RecoveryPlan{}, errors.New("recovery requires durable work units")
	}
	if len(state.Tasks) != len(run.WorkUnits) {
		return RecoveryPlan{}, fmt.Errorf(
			"recovery task count mismatch: runstate=%d control-plane=%d",
			len(run.WorkUnits),
			len(state.Tasks),
		)
	}

	mutating, kindsComplete := mutationProfile(run)
	if mutating {
		if !kindsComplete {
			return RecoveryPlan{}, errors.New("mutating recovery requires durable mutation kinds")
		}
		phase := runcheckpoint.Phase(state.Pipeline.CheckpointPhase)
		if !resumableCheckpoint(string(phase)) {
			return RecoveryPlan{}, fmt.Errorf(
				"mutating recovery requires source/final integration checkpoint: %q",
				state.Pipeline.CheckpointPhase,
			)
		}
		if !input.CheckpointAvailable {
			return RecoveryPlan{}, errors.New("mutating recovery checkpoint is unavailable or invalid")
		}
		statuses := make(map[string]runstate.WorkUnitStatus, len(run.WorkUnits))
		for _, unit := range run.WorkUnits {
			if _, ok := state.Tasks[unit.ID]; !ok {
				return RecoveryPlan{}, fmt.Errorf("control-plane is missing task %s", unit.ID)
			}
			switch phase {
			case runcheckpoint.PhaseSourceIntegrated:
				if unit.MutationKind == "tests-only" {
					statuses[unit.ID] = runstate.WorkUnitQueued
				} else {
					statuses[unit.ID] = runstate.WorkUnitCompleted
				}
			case runcheckpoint.PhaseFinalIntegrated:
				statuses[unit.ID] = runstate.WorkUnitCompleted
			default:
				return RecoveryPlan{}, fmt.Errorf("unsupported mutation recovery checkpoint: %s", phase)
			}
		}
		return RecoveryPlan{
			Status:          runstate.StatusPaused,
			CheckpointPhase: string(phase),
			TaskStatuses:    statuses,
		}, nil
	}

	statuses := make(map[string]runstate.WorkUnitStatus, len(run.WorkUnits))
	hasFailure := false
	for _, unit := range run.WorkUnits {
		task, ok := state.Tasks[unit.ID]
		if !ok {
			return RecoveryPlan{}, fmt.Errorf("control-plane is missing task %s", unit.ID)
		}
		switch task.Status {
		case controlplane.TaskCompleted:
			statuses[unit.ID] = runstate.WorkUnitCompleted
		case controlplane.TaskFailed:
			statuses[unit.ID] = runstate.WorkUnitFailed
			hasFailure = true
		case controlplane.TaskBlocked:
			statuses[unit.ID] = runstate.WorkUnitBlocked
			hasFailure = true
		case controlplane.TaskCancelled:
			statuses[unit.ID] = runstate.WorkUnitCancelled
			hasFailure = true
		case controlplane.TaskQueued, controlplane.TaskAssigned, controlplane.TaskRunning, controlplane.TaskWaiting:
			statuses[unit.ID] = runstate.WorkUnitQueued
		default:
			return RecoveryPlan{}, fmt.Errorf(
				"unsupported control-plane task status for %s: %s",
				unit.ID,
				task.Status,
			)
		}
	}
	target := runstate.StatusPaused
	if hasFailure {
		target = runstate.StatusFailed
	}
	return RecoveryPlan{
		Status:       target,
		TaskStatuses: statuses,
	}, nil
}
