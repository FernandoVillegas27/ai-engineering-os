package runcontrol

import (
	"testing"

	"github.com/FernandoVillegas27/ai-engineering-os/core/controlplane"
	"github.com/FernandoVillegas27/ai-engineering-os/core/runstate"
)

func TestDeriveCapabilitiesForRunningReadOnlyRun(t *testing.T) {
	value := DeriveCapabilities(CapabilityInput{
		Run: runstate.Run{
			ID: "run-1", Status: runstate.StatusRunning, Intent: "research",
		},
		ControlPlane: controlplane.State{
			RunID: "run-1", Status: controlplane.RunRunning,
		},
	})
	if !value.Consistent || !value.CanPause || !value.CanCancel || value.CanResume || value.CanRetry {
		t.Fatalf("controls=%#v", value)
	}
	if value.PauseBoundary != "task-boundary" {
		t.Fatalf("pause boundary=%q", value.PauseBoundary)
	}
}

func TestDeriveCapabilitiesForPausedMutationRequiresValidCheckpoint(t *testing.T) {
	run := runstate.Run{
		ID: "run-mutation", Status: runstate.StatusPaused, Intent: "feature",
		WorkUnits: []runstate.WorkUnit{
			{ID: "backend", Mutating: true, MutationKind: "source", Status: runstate.WorkUnitCompleted},
			{ID: "qa", Mutating: true, MutationKind: "tests-only", Status: runstate.WorkUnitCompleted},
		},
	}
	state := controlplane.State{
		RunID: "run-mutation", Status: controlplane.RunPaused,
		Pipeline: controlplane.PipelineState{CheckpointPhase: "final-integrated"},
	}
	missing := DeriveCapabilities(CapabilityInput{Run: run, ControlPlane: state})
	if missing.CanResume || !missing.CanCancel {
		t.Fatalf("missing checkpoint controls=%#v", missing)
	}
	if missing.Reason == "" {
		t.Fatal("missing checkpoint should explain why resume is unavailable")
	}

	ready := DeriveCapabilities(CapabilityInput{
		Run: run, ControlPlane: state, CheckpointAvailable: true,
	})
	if !ready.CanResume || ready.ResumeFrom != "final-integrated" {
		t.Fatalf("ready controls=%#v", ready)
	}
}

func TestDeriveCapabilitiesRejectsLegacyMutationKindForPause(t *testing.T) {
	value := DeriveCapabilities(CapabilityInput{
		Run: runstate.Run{
			ID: "run-legacy", Status: runstate.StatusRunning, Intent: "feature",
			WorkUnits: []runstate.WorkUnit{{ID: "backend", Mutating: true}},
		},
		ControlPlane: controlplane.State{
			RunID: "run-legacy", Status: controlplane.RunRunning,
		},
	})
	if value.CanPause || !value.CanCancel {
		t.Fatalf("controls=%#v", value)
	}
}

func TestDeriveCapabilitiesListsOnlyRetryableReadOnlyTasks(t *testing.T) {
	value := DeriveCapabilities(CapabilityInput{
		Run: runstate.Run{
			ID: "run-failed", Status: runstate.StatusFailed, Intent: "research",
			WorkUnits: []runstate.WorkUnit{
				{ID: "research", Status: runstate.WorkUnitCompleted},
				{ID: "architect", Status: runstate.WorkUnitFailed, DependsOn: []string{"research"}},
				{ID: "review", Status: runstate.WorkUnitBlocked, DependsOn: []string{"architect"}},
			},
		},
		ControlPlane: controlplane.State{
			RunID: "run-failed", Status: controlplane.RunFailed,
		},
	})
	if !value.CanRetry || len(value.RetryableTaskIDs) != 1 || value.RetryableTaskIDs[0] != "architect" {
		t.Fatalf("controls=%#v", value)
	}
}

func TestDeriveCapabilitiesDisablesControlsOnStateMismatch(t *testing.T) {
	value := DeriveCapabilities(CapabilityInput{
		Run: runstate.Run{ID: "run-1", Status: runstate.StatusPaused},
		ControlPlane: controlplane.State{RunID: "run-1", Status: controlplane.RunRunning},
	})
	if value.Consistent || value.CanPause || value.CanResume || value.CanCancel || value.CanRetry {
		t.Fatalf("controls=%#v", value)
	}
}


func TestDeriveCapabilitiesKeepsPlannedRunFailClosed(t *testing.T) {
	value := DeriveCapabilities(CapabilityInput{
		Run: runstate.Run{
			ID: "run-planned", Status: runstate.StatusPlanned,
		},
		ControlPlane: controlplane.State{
			RunID: "run-planned", Status: controlplane.RunRunning,
		},
	})
	if value.Consistent || value.CanPause || value.CanResume || value.CanCancel || value.CanRetry {
		t.Fatalf("controls=%#v", value)
	}
}
