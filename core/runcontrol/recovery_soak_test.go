package runcontrol

import (
	"fmt"
	"testing"

	"github.com/FernandoVillegas27/ai-engineering-os/core/controlplane"
	"github.com/FernandoVillegas27/ai-engineering-os/core/runstate"
)

func TestDeriveRecoveryLargeReadOnlyRunSoak(t *testing.T) {
	if testing.Short() {
		t.Skip("soak-style recovery derivation test")
	}

	const taskCount = 256
	run := runstate.Run{
		ID:     "run-recovery-soak",
		Status: runstate.StatusRunning,
	}
	state := controlplane.State{
		RunID:  run.ID,
		Status: controlplane.RunRunning,
		Tasks:  make(map[string]controlplane.TaskState, taskCount),
	}

	for i := 0; i < taskCount; i++ {
		id := fmt.Sprintf("task-%03d", i)
		status := controlplane.TaskRunning
		switch i % 4 {
		case 0:
			status = controlplane.TaskCompleted
		case 1:
			status = controlplane.TaskAssigned
		case 2:
			status = controlplane.TaskWaiting
		case 3:
			status = controlplane.TaskQueued
		}
		run.WorkUnits = append(run.WorkUnits, runstate.WorkUnit{
			ID:      id,
			AgentID: fmt.Sprintf("agent-%02d", i%16),
			Task:    "soak task",
			Status:  runstate.WorkUnitRunning,
		})
		state.Tasks[id] = controlplane.TaskState{
			ID:     id,
			Status: status,
		}
	}

	plan, err := DeriveRecovery(RecoveryInput{Run: run, ControlPlane: state})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Status != runstate.StatusPaused {
		t.Fatalf("status=%s want=%s", plan.Status, runstate.StatusPaused)
	}
	if len(plan.TaskStatuses) != taskCount {
		t.Fatalf("task statuses=%d want=%d", len(plan.TaskStatuses), taskCount)
	}
	for i := 0; i < taskCount; i++ {
		id := fmt.Sprintf("task-%03d", i)
		want := runstate.WorkUnitQueued
		if i%4 == 0 {
			want = runstate.WorkUnitCompleted
		}
		if got := plan.TaskStatuses[id]; got != want {
			t.Fatalf("%s=%s want=%s", id, got, want)
		}
	}
}

func TestDeriveRecoveryLargeRunPreservesFailureEvidence(t *testing.T) {
	const taskCount = 128
	run := runstate.Run{
		ID:     "run-recovery-failure-soak",
		Status: runstate.StatusRunning,
	}
	state := controlplane.State{
		RunID:  run.ID,
		Status: controlplane.RunRunning,
		Tasks:  make(map[string]controlplane.TaskState, taskCount),
	}

	for i := 0; i < taskCount; i++ {
		id := fmt.Sprintf("task-%03d", i)
		status := controlplane.TaskRunning
		if i == taskCount-1 {
			status = controlplane.TaskFailed
		}
		run.WorkUnits = append(run.WorkUnits, runstate.WorkUnit{
			ID:      id,
			AgentID: "agent",
			Task:    "soak task",
			Status:  runstate.WorkUnitRunning,
		})
		state.Tasks[id] = controlplane.TaskState{ID: id, Status: status}
	}

	plan, err := DeriveRecovery(RecoveryInput{Run: run, ControlPlane: state})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Status != runstate.StatusFailed {
		t.Fatalf("status=%s want=%s", plan.Status, runstate.StatusFailed)
	}
	if got := plan.TaskStatuses["task-127"]; got != runstate.WorkUnitFailed {
		t.Fatalf("failed evidence=%s", got)
	}
}
