package main

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/controlplane"
	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/runstate"
	"github.com/FernandoVillegas27/ai-engineering-os/core/toolapproval"
)

func TestRunToolApprovalRequestThenGrantPersistsEvidenceAndEvents(t *testing.T) {
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("AI_ENGINEERING_OS_HOME", repoRoot)
	root := t.TempDir()
	stateDir := filepath.Join(root, "runs")
	eventsDir := filepath.Join(root, "events")
	approvalDir := filepath.Join(root, "approvals")

	runStore, err := runstate.NewStore(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	run, err := runstate.New(runstate.NewRunRequest{
		Task: "review repository",
		ProjectID: "project-1",
		WorkUnits: []execution.WorkUnit{{
			ID: "reviewer-1",
			AgentID: "reviewer",
			Task: "review repository",
			Tools: []string{"github"},
			ToolOperations: map[string][]string{
				"github": {"read-repository"},
			},
		}},
	}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := runStore.Save(run); err != nil {
		t.Fatal(err)
	}

	eventStore, err := controlplane.NewEventStore(eventsDir)
	if err != nil {
		t.Fatal(err)
	}
	started, err := controlplane.NewEvent(controlplane.NewEventRequest{
		RunID: run.ID,
		Type: controlplane.EventRunStarted,
		Data: map[string]any{"source": "test"},
	}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := eventStore.Append(context.Background(), started); err != nil {
		t.Fatal(err)
	}

	common := []string{
		run.ID,
		"--task", "reviewer-1",
		"--tool", "github",
		"--operation", "create-pull-request",
		"--state-dir", stateDir,
		"--events-dir", eventsDir,
	}
	if err := runRunToolApprovalRequest(common); err != nil {
		t.Fatal(err)
	}
	grant := append(append([]string(nil), common...), "--approval-dir", approvalDir, "--expires-in", "10m")
	if err := runRunToolApprovalResolve(grant, true); err != nil {
		t.Fatal(err)
	}

	approvalStore, err := toolapproval.NewStore(approvalDir)
	if err != nil {
		t.Fatal(err)
	}
	values, err := approvalStore.List(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 1 {
		t.Fatalf("approvals=%#v", values)
	}
	if !toolapproval.Grants(
		values[0], run.ID, "reviewer-1", "github", "create-pull-request", time.Now().UTC(),
	) {
		t.Fatalf("approval does not grant expected scope: %#v", values[0])
	}

	events, err := eventStore.List(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 ||
		events[1].Type != controlplane.EventApprovalRequested ||
		events[2].Type != controlplane.EventApprovalGranted {
		t.Fatalf("events=%#v", events)
	}
}

func TestRunToolApprovalRejectsToolNotDeclaredForTask(t *testing.T) {
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("AI_ENGINEERING_OS_HOME", repoRoot)
	root := t.TempDir()
	stateDir := filepath.Join(root, "runs")
	eventsDir := filepath.Join(root, "events")

	runStore, err := runstate.NewStore(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	run, err := runstate.New(runstate.NewRunRequest{
		Task: "inspect project",
		ProjectID: "project-1",
		WorkUnits: []execution.WorkUnit{{
			ID: "architect-1",
			AgentID: "architect",
			Task: "inspect project",
			Tools: []string{"filesystem"},
			ToolOperations: map[string][]string{
				"filesystem": {"read-files"},
			},
		}},
	}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := runStore.Save(run); err != nil {
		t.Fatal(err)
	}

	eventStore, err := controlplane.NewEventStore(eventsDir)
	if err != nil {
		t.Fatal(err)
	}
	started, err := controlplane.NewEvent(controlplane.NewEventRequest{
		RunID: run.ID, Type: controlplane.EventRunStarted, Data: map[string]bool{"ok": true},
	}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := eventStore.Append(context.Background(), started); err != nil {
		t.Fatal(err)
	}

	err = runRunToolApprovalRequest([]string{
		run.ID,
		"--task", "architect-1",
		"--tool", "github",
		"--operation", "create-pull-request",
		"--state-dir", stateDir,
		"--events-dir", eventsDir,
	})
	if err == nil {
		t.Fatal("expected undeclared task tool rejection")
	}
}

func TestToolApprovalPendingLifecycleIsExactByOperation(t *testing.T) {
	now := time.Now().UTC()
	makeEvent := func(typ controlplane.EventType, operation string) controlplane.Event {
		t.Helper()
		event, err := controlplane.NewEvent(controlplane.NewEventRequest{
			RunID: "run-1", TaskID: "task-1", Type: typ,
			Data: map[string]any{
				"kind": "tool-operation",
				"tool_id": "github",
				"operation": operation,
			},
		}, now)
		if err != nil {
			t.Fatal(err)
		}
		return event
	}
	events := []controlplane.Event{
		makeEvent(controlplane.EventApprovalRequested, "create-pull-request"),
		makeEvent(controlplane.EventApprovalRequested, "create-commit"),
		makeEvent(controlplane.EventApprovalGranted, "create-pull-request"),
	}
	if toolApprovalPending(events, "task-1", "github", "create-pull-request") {
		t.Fatal("granted operation must no longer be pending")
	}
	if !toolApprovalPending(events, "task-1", "github", "create-commit") {
		t.Fatal("sibling operation must remain pending")
	}
}
