package main

import (
	"context"
	"testing"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/artifact"
	"github.com/FernandoVillegas27/ai-engineering-os/core/controlplane"
	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/runstate"
)

func retryFixtureRun(t *testing.T, stateDir, eventsDir string) runstate.Run {
	t.Helper()
	now := time.Now().UTC()
	parent, err := runstate.New(runstate.NewRunRequest{
		Task:        "analyze architecture",
		Intent:      "research",
		ProjectID:   "project",
		ProjectRoot: t.TempDir(),
		Runtime: runstate.RuntimeSelection{
			Provider:    "dry-run",
			ModelPolicy: "auto",
		},
		WorkUnits: []execution.WorkUnit{
			{
				ID: "research", AgentID: "researcher", Task: "analyze architecture",
				Permissions: execution.PermissionEnvelope{
					SourceCode: "read", Tests: "read", GitCommit: "deny",
					GitPush: "deny", Secrets: "deny", Production: "deny",
				},
				Mode: execution.ModeReadOnly,
			},
			{
				ID: "architect", AgentID: "architect", Task: "analyze architecture",
				Permissions: execution.PermissionEnvelope{
					SourceCode: "read", Tests: "read", GitCommit: "deny",
					GitPush: "deny", Secrets: "deny", Production: "deny",
				},
				Mode: execution.ModeReadOnly,
			},
		},
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	parent.WorkUnits[0].Status = runstate.WorkUnitCompleted
	parent.WorkUnits[1].Status = runstate.WorkUnitFailed
	parent.WorkUnits[1].DependsOn = []string{"research"}
	if err := runstate.Transition(&parent, runstate.StatusRunning, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := runstate.Transition(&parent, runstate.StatusFailed, now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	store, err := runstate.NewStore(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(parent); err != nil {
		t.Fatal(err)
	}

	eventStore, err := controlplane.NewEventStore(eventsDir)
	if err != nil {
		t.Fatal(err)
	}
	started, err := controlplane.NewEvent(controlplane.NewEventRequest{
		RunID: parent.ID, Type: controlplane.EventRunStarted,
		Data: map[string]any{"source": "test"},
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := eventStore.Append(context.Background(), started); err != nil {
		t.Fatal(err)
	}
	failed, err := controlplane.NewEvent(controlplane.NewEventRequest{
		RunID: parent.ID, Type: controlplane.EventRunFailed,
		Data: map[string]any{"source": "test"},
	}, now.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := eventStore.Append(context.Background(), failed); err != nil {
		t.Fatal(err)
	}
	return parent
}

func TestRunRetryCreatesCompletedChildRunAndKeepsParentImmutable(t *testing.T) {
	base := t.TempDir()
	stateDir := base + "/runs"
	eventsDir := base + "/events"
	artifactsDir := base + "/artifacts"
	messagesDir := base + "/messages"
	controlDir := base + "/control"

	parent := retryFixtureRun(t, stateDir, eventsDir)

	if err := runRunRetry([]string{
		parent.ID, "architect",
		"--state-dir", stateDir,
		"--events-dir", eventsDir,
		"--artifacts-dir", artifactsDir,
		"--messages-dir", messagesDir,
		"--control-dir", controlDir,
	}); err != nil {
		t.Fatal(err)
	}

	store, err := runstate.NewStore(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	runs, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	var child *runstate.Run
	for i := range runs {
		if runs[i].ParentRunID == parent.ID {
			value := runs[i]
			child = &value
			break
		}
	}
	if child == nil {
		t.Fatalf("child run not found: %#v", runs)
	}
	if child.RetryOfTaskID != "architect" || child.Status != runstate.StatusCompleted {
		t.Fatalf("child = %#v", child)
	}
	if len(child.WorkUnits) != 1 || child.WorkUnits[0].Status != runstate.WorkUnitCompleted {
		t.Fatalf("child work units = %#v", child.WorkUnits)
	}

	reloadedParent, err := store.Load(parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloadedParent.Status != runstate.StatusFailed ||
		reloadedParent.WorkUnits[1].Status != runstate.WorkUnitFailed {
		t.Fatalf("parent mutated = %#v", reloadedParent)
	}

	eventStore, err := controlplane.NewEventStore(eventsDir)
	if err != nil {
		t.Fatal(err)
	}
	events, err := eventStore.List(child.ID)
	if err != nil {
		t.Fatal(err)
	}
	state, err := controlplane.Project(events)
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != controlplane.RunCompleted ||
		state.ParentRunID != parent.ID ||
		state.RetryOfTaskID != "architect" {
		t.Fatalf("child control-plane state = %#v", state)
	}

	artifactStore, err := artifact.NewStore(artifactsDir)
	if err != nil {
		t.Fatal(err)
	}
	values, err := artifactStore.ListRun(child.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 1 || values[0].ProducerAgent != "architect" {
		t.Fatalf("child artifacts = %#v", values)
	}
}

func TestRunRetryRejectsMutatingParentRun(t *testing.T) {
	base := t.TempDir()
	stateDir := base + "/runs"
	eventsDir := base + "/events"
	parent := retryFixtureRun(t, stateDir, eventsDir)

	store, err := runstate.NewStore(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	parent.WorkUnits[0].Mutating = true
	if err := store.Save(parent); err != nil {
		t.Fatal(err)
	}
	if err := runRunRetry([]string{
		parent.ID, "architect",
		"--state-dir", stateDir,
		"--events-dir", eventsDir,
	}); err == nil {
		t.Fatal("expected mutating parent rejection")
	}
}

func TestRunRetryRejectsIncompleteDependency(t *testing.T) {
	base := t.TempDir()
	stateDir := base + "/runs"
	eventsDir := base + "/events"
	parent := retryFixtureRun(t, stateDir, eventsDir)

	store, err := runstate.NewStore(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	parent.WorkUnits[0].Status = runstate.WorkUnitFailed
	if err := store.Save(parent); err != nil {
		t.Fatal(err)
	}
	if err := runRunRetry([]string{
		parent.ID, "architect",
		"--state-dir", stateDir,
		"--events-dir", eventsDir,
	}); err == nil {
		t.Fatal("expected incomplete dependency rejection")
	}
}

func TestParseRunRetryArgs(t *testing.T) {
	options, err := parseRunRetryArgs([]string{
		"run-parent", "work-1",
		"--state-dir", "./runs",
		"--events-dir", "./events",
		"--json",
	})
	if err != nil {
		t.Fatal(err)
	}
	if options.runID != "run-parent" || options.taskID != "work-1" ||
		options.stateDir != "./runs" || options.eventsDir != "./events" || !options.json {
		t.Fatalf("options=%#v", options)
	}
}
