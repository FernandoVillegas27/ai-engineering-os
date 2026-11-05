package runcontrol

import (
	"testing"

	"github.com/FernandoVillegas27/ai-engineering-os/core/controlplane"
	"github.com/FernandoVillegas27/ai-engineering-os/core/runstate"
)

func TestDeriveRecoveryReadOnlyRequeuesActiveTasks(t *testing.T) {
	run := runstate.Run{
		ID: "run-readonly", Status: runstate.StatusRunning,
		WorkUnits: []runstate.WorkUnit{
			{ID: "research", Status: runstate.WorkUnitQueued},
			{ID: "architect", Status: runstate.WorkUnitQueued, DependsOn: []string{"research"}},
		},
	}
	state := controlplane.State{
		RunID: "run-readonly", Status: controlplane.RunRunning,
		Tasks: map[string]controlplane.TaskState{
			"research": {ID: "research", Status: controlplane.TaskCompleted},
			"architect": {ID: "architect", Status: controlplane.TaskRunning},
		},
	}
	plan, err := DeriveRecovery(RecoveryInput{Run: run, ControlPlane: state})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Status != runstate.StatusPaused ||
		plan.TaskStatuses["research"] != runstate.WorkUnitCompleted ||
		plan.TaskStatuses["architect"] != runstate.WorkUnitQueued {
		t.Fatalf("plan=%#v", plan)
	}
}

func TestDeriveRecoveryReadOnlyWithFailureBecomesFailed(t *testing.T) {
	run := runstate.Run{
		ID: "run-failed", Status: runstate.StatusRunning,
		WorkUnits: []runstate.WorkUnit{
			{ID: "research"},
			{ID: "architect", DependsOn: []string{"research"}},
		},
	}
	state := controlplane.State{
		RunID: "run-failed", Status: controlplane.RunRunning,
		Tasks: map[string]controlplane.TaskState{
			"research": {ID: "research", Status: controlplane.TaskFailed},
			"architect": {ID: "architect", Status: controlplane.TaskWaiting},
		},
	}
	plan, err := DeriveRecovery(RecoveryInput{Run: run, ControlPlane: state})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Status != runstate.StatusFailed ||
		plan.TaskStatuses["research"] != runstate.WorkUnitFailed ||
		plan.TaskStatuses["architect"] != runstate.WorkUnitQueued {
		t.Fatalf("plan=%#v", plan)
	}
}

func TestDeriveRecoveryMutationFromSourceCheckpoint(t *testing.T) {
	run := runstate.Run{
		ID: "run-mutation", Status: runstate.StatusRunning,
		WorkUnits: []runstate.WorkUnit{
			{ID: "architect", Status: runstate.WorkUnitCompleted},
			{ID: "backend", Mutating: true, MutationKind: "source", Status: runstate.WorkUnitCompleted},
			{ID: "qa", Mutating: true, MutationKind: "tests-only", Status: runstate.WorkUnitRunning},
		},
	}
	state := controlplane.State{
		RunID: "run-mutation", Status: controlplane.RunRunning,
		Pipeline: controlplane.PipelineState{CheckpointPhase: "source-integrated"},
		Tasks: map[string]controlplane.TaskState{
			"architect": {ID: "architect", Status: controlplane.TaskCompleted},
			"backend": {ID: "backend", Status: controlplane.TaskCompleted},
			"qa": {ID: "qa", Status: controlplane.TaskRunning},
		},
	}
	plan, err := DeriveRecovery(RecoveryInput{
		Run: run, ControlPlane: state, CheckpointAvailable: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Status != runstate.StatusPaused ||
		plan.CheckpointPhase != "source-integrated" ||
		plan.TaskStatuses["architect"] != runstate.WorkUnitCompleted ||
		plan.TaskStatuses["backend"] != runstate.WorkUnitCompleted ||
		plan.TaskStatuses["qa"] != runstate.WorkUnitQueued {
		t.Fatalf("plan=%#v", plan)
	}
}

func TestDeriveRecoveryMutationFromFinalCheckpointCompletesAllTasks(t *testing.T) {
	run := runstate.Run{
		ID: "run-final", Status: runstate.StatusRunning,
		WorkUnits: []runstate.WorkUnit{
			{ID: "backend", Mutating: true, MutationKind: "source"},
			{ID: "qa", Mutating: true, MutationKind: "tests-only"},
		},
	}
	state := controlplane.State{
		RunID: "run-final", Status: controlplane.RunRunning,
		Pipeline: controlplane.PipelineState{CheckpointPhase: "final-integrated"},
		Tasks: map[string]controlplane.TaskState{
			"backend": {ID: "backend", Status: controlplane.TaskCompleted},
			"qa": {ID: "qa", Status: controlplane.TaskCompleted},
		},
	}
	plan, err := DeriveRecovery(RecoveryInput{
		Run: run, ControlPlane: state, CheckpointAvailable: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Status != runstate.StatusPaused ||
		plan.TaskStatuses["backend"] != runstate.WorkUnitCompleted ||
		plan.TaskStatuses["qa"] != runstate.WorkUnitCompleted {
		t.Fatalf("plan=%#v", plan)
	}
}

func TestDeriveRecoveryMutationRejectsMissingCheckpointOrKind(t *testing.T) {
	run := runstate.Run{
		ID: "run-mutation", Status: runstate.StatusRunning,
		WorkUnits: []runstate.WorkUnit{
			{ID: "backend", Mutating: true, MutationKind: "source"},
		},
	}
	state := controlplane.State{
		RunID: "run-mutation", Status: controlplane.RunRunning,
		Pipeline: controlplane.PipelineState{CheckpointPhase: "source-integrated"},
		Tasks: map[string]controlplane.TaskState{
			"backend": {ID: "backend", Status: controlplane.TaskRunning},
		},
	}
	if _, err := DeriveRecovery(RecoveryInput{Run: run, ControlPlane: state}); err == nil {
		t.Fatal("expected invalid checkpoint rejection")
	}
	run.WorkUnits[0].MutationKind = ""
	if _, err := DeriveRecovery(RecoveryInput{
		Run: run, ControlPlane: state, CheckpointAvailable: true,
	}); err == nil {
		t.Fatal("expected missing mutation kind rejection")
	}
}

func TestDeriveRecoveryRejectsStateOrTaskMismatch(t *testing.T) {
	run := runstate.Run{
		ID: "run-1", Status: runstate.StatusRunning,
		WorkUnits: []runstate.WorkUnit{{ID: "task-1"}},
	}
	if _, err := DeriveRecovery(RecoveryInput{
		Run: run,
		ControlPlane: controlplane.State{
			RunID: "run-1", Status: controlplane.RunPaused,
			Tasks: map[string]controlplane.TaskState{"task-1": {ID: "task-1"}},
		},
	}); err == nil {
		t.Fatal("expected status mismatch rejection")
	}
	if _, err := DeriveRecovery(RecoveryInput{
		Run: run,
		ControlPlane: controlplane.State{
			RunID: "run-1", Status: controlplane.RunRunning,
			Tasks: map[string]controlplane.TaskState{},
		},
	}); err == nil {
		t.Fatal("expected task mismatch rejection")
	}
}
