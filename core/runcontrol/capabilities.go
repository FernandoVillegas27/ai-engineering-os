package runcontrol

import (
	"fmt"
	"sort"

	"github.com/FernandoVillegas27/ai-engineering-os/core/controlplane"
	"github.com/FernandoVillegas27/ai-engineering-os/core/runcheckpoint"
	"github.com/FernandoVillegas27/ai-engineering-os/core/runstate"
)

type Capabilities struct {
	Consistent        bool     `json:"consistent"`
	CanPause          bool     `json:"can_pause"`
	CanResume         bool     `json:"can_resume"`
	CanCancel         bool     `json:"can_cancel"`
	CanRetry          bool     `json:"can_retry"`
	PauseBoundary     string   `json:"pause_boundary,omitempty"`
	ResumeFrom        string   `json:"resume_from,omitempty"`
	RetryableTaskIDs  []string `json:"retryable_task_ids,omitempty"`
	Reason            string   `json:"reason,omitempty"`
}

type CapabilityInput struct {
	Run                 runstate.Run
	ControlPlane        controlplane.State
	CheckpointAvailable bool
}

func DeriveCapabilities(input CapabilityInput) Capabilities {
	run := input.Run
	state := input.ControlPlane
	out := Capabilities{}

	if run.ID == "" || state.RunID == "" || run.ID != state.RunID {
		out.Reason = "runstate/control-plane identity mismatch"
		return out
	}
	if !statusesAgree(run.Status, state.Status) {
		out.Reason = fmt.Sprintf(
			"runstate/control-plane status mismatch: %s/%s",
			run.Status,
			state.Status,
		)
		return out
	}
	out.Consistent = true

	mutating, mutationKindsComplete := mutationProfile(run)
	switch run.Status {
	case runstate.StatusRunning:
		out.CanCancel = true
		if !mutating {
			out.CanPause = true
			out.PauseBoundary = "task-boundary"
		} else if mutationKindsComplete {
			out.CanPause = true
			out.PauseBoundary = "integration-checkpoint"
		}
	case runstate.StatusPaused:
		out.CanCancel = true
		if run.Intent == "" {
			out.Reason = "paused run has no durable intent"
			return out
		}
		if !mutating {
			out.CanResume = true
			out.ResumeFrom = "durable-task-graph"
		} else if mutationKindsComplete && resumableCheckpoint(state.Pipeline.CheckpointPhase) && input.CheckpointAvailable {
			out.CanResume = true
			out.ResumeFrom = state.Pipeline.CheckpointPhase
		}
	case runstate.StatusFailed:
		if !mutating {
			out.RetryableTaskIDs = retryableTasks(run)
			out.CanRetry = len(out.RetryableTaskIDs) > 0
		}
	}
	return out
}

func mutationProfile(run runstate.Run) (mutating bool, kindsComplete bool) {
	kindsComplete = true
	for _, unit := range run.WorkUnits {
		if !unit.Mutating {
			continue
		}
		mutating = true
		switch unit.MutationKind {
		case "source", "tests-only":
		default:
			kindsComplete = false
		}
	}
	return mutating, kindsComplete
}

func retryableTasks(run runstate.Run) []string {
	status := map[string]runstate.WorkUnitStatus{}
	for _, unit := range run.WorkUnits {
		status[unit.ID] = unit.Status
	}
	var out []string
	for _, unit := range run.WorkUnits {
		if unit.Status != runstate.WorkUnitFailed && unit.Status != runstate.WorkUnitBlocked {
			continue
		}
		ready := true
		for _, dependency := range unit.DependsOn {
			if status[dependency] != runstate.WorkUnitCompleted {
				ready = false
				break
			}
		}
		if ready {
			out = append(out, unit.ID)
		}
	}
	sort.Strings(out)
	return out
}

func resumableCheckpoint(value string) bool {
	switch runcheckpoint.Phase(value) {
	case runcheckpoint.PhaseSourceIntegrated, runcheckpoint.PhaseFinalIntegrated:
		return true
	default:
		return false
	}
}

func statusesAgree(runStatus runstate.Status, cpStatus controlplane.RunStatus) bool {
	switch runStatus {
	case runstate.StatusPlanned:
		return cpStatus == controlplane.RunPlanned
	case runstate.StatusRunning:
		return cpStatus == controlplane.RunRunning
	case runstate.StatusPaused:
		return cpStatus == controlplane.RunPaused
	case runstate.StatusCompleted:
		return cpStatus == controlplane.RunCompleted
	case runstate.StatusFailed:
		return cpStatus == controlplane.RunFailed
	case runstate.StatusCancelled:
		return cpStatus == controlplane.RunCancelled
	default:
		return false
	}
}
