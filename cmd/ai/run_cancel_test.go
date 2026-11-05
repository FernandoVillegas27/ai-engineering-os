package main

import (
	"context"
	"testing"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/controlplane"
	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/runcontrol"
	"github.com/FernandoVillegas27/ai-engineering-os/core/runstate"
	"github.com/FernandoVillegas27/ai-engineering-os/core/scheduler"
)

func TestParseRunCancelArgs(t *testing.T) {
	options, err := parseRunCancelArgs([]string{
		"run-123",
		"--control-dir", "./control",
		"--state-dir", "./runs",
		"--events-dir", "./events",
		"--json",
	})
	if err != nil {
		t.Fatal(err)
	}
	if options.runID != "run-123" || !options.json ||
		options.controlDir != "./control" ||
		options.stateDir != "./runs" ||
		options.eventsDir != "./events" {
		t.Fatalf("options=%#v", options)
	}
}

func TestRunCancelPersistsRequestAndControlPlaneEvent(t *testing.T) {
	base := t.TempDir()
	stateDir := base + "/runs"
	eventsDir := base + "/events"
	controlDir := base + "/control"

	runStore, err := runstate.NewStore(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	run, err := runstate.New(runstate.NewRunRequest{
		Task:        "long task",
		ProjectID:   "project",
		ProjectRoot: base,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := runstate.Transition(&run, runstate.StatusRunning, now.Add(time.Second)); err != nil {
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
		Type:  controlplane.EventRunStarted,
		Data:  map[string]bool{"ok": true},
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := eventStore.Append(context.Background(), started); err != nil {
		t.Fatal(err)
	}

	if err := runRunCancel([]string{
		run.ID,
		"--control-dir", controlDir,
		"--state-dir", stateDir,
		"--events-dir", eventsDir,
	}); err != nil {
		t.Fatal(err)
	}

	controlStore, err := runcontrol.NewStore(controlDir)
	if err != nil {
		t.Fatal(err)
	}
	requested, err := controlStore.CancelRequested(run.ID)
	if err != nil || !requested {
		t.Fatalf("requested=%t err=%v", requested, err)
	}

	events, err := eventStore.List(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[1].Type != controlplane.EventRunCancelRequested {
		t.Fatalf("events=%#v", events)
	}
	state, err := controlplane.Project(events)
	if err != nil {
		t.Fatal(err)
	}
	if !state.CancelRequested || state.Status != controlplane.RunRunning {
		t.Fatalf("state=%#v", state)
	}

	// Idempotent repeated request must not duplicate the event.
	if err := runRunCancel([]string{
		run.ID,
		"--control-dir", controlDir,
		"--state-dir", stateDir,
		"--events-dir", eventsDir,
	}); err != nil {
		t.Fatal(err)
	}
	events, err = eventStore.List(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("events=%#v", events)
	}
}

func TestRunCancelRejectsTerminalRun(t *testing.T) {
	base := t.TempDir()
	store, err := runstate.NewStore(base + "/runs")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	run, err := runstate.New(runstate.NewRunRequest{
		Task: "done", ProjectID: "project", ProjectRoot: base,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := runstate.Transition(&run, runstate.StatusRunning, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := runstate.Transition(&run, runstate.StatusCompleted, now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(run); err != nil {
		t.Fatal(err)
	}
	if err := runRunCancel([]string{run.ID, "--state-dir", base + "/runs"}); err == nil {
		t.Fatal("expected terminal run cancellation rejection")
	}
}


func TestRunCancelRejectsMismatchedEventsDirectory(t *testing.T) {
	base := t.TempDir()
	stateDir := base + "/runs"
	runStore, err := runstate.NewStore(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	run, err := runstate.New(runstate.NewRunRequest{
		Task: "running", ProjectID: "project", ProjectRoot: base,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := runstate.Transition(&run, runstate.StatusRunning, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := runStore.Save(run); err != nil {
		t.Fatal(err)
	}

	err = runRunCancel([]string{
		run.ID,
		"--state-dir", stateDir,
		"--events-dir", base + "/wrong-events",
		"--control-dir", base + "/control",
	})
	if err == nil {
		t.Fatal("expected mismatched events directory rejection")
	}
	controlStore, openErr := runcontrol.NewStore(base + "/control")
	if openErr != nil {
		t.Fatal(openErr)
	}
	requested, requestErr := controlStore.CancelRequested(run.ID)
	if requestErr != nil {
		t.Fatal(requestErr)
	}
	if requested {
		t.Fatal("cancel request was persisted before event-stream validation")
	}

	eventStore, openErr := controlplane.NewEventStore(base + "/wrong-events")
	if openErr != nil {
		t.Fatal(openErr)
	}
	events, listErr := eventStore.List(run.ID)
	if listErr != nil {
		t.Fatal(listErr)
	}
	if len(events) != 0 {
		t.Fatalf("invalid event stream created: %#v", events)
	}
}


func TestRunCancelCancelsPausedRunSynchronously(t *testing.T) {
	base := t.TempDir()
	stateDir := base + "/runs"
	eventsDir := base + "/events"
	controlDir := base + "/control"
	now := time.Now().UTC()

	run, err := runstate.New(runstate.NewRunRequest{
		Task: "paused work", Intent: "research", ProjectID: "project", ProjectRoot: base,
		Runtime: runstate.RuntimeSelection{Provider: "dry-run", ModelPolicy: "auto"},
		WorkUnits: []execution.WorkUnit{{
			ID: "research", AgentID: "researcher", Task: "paused work",
			Permissions: execution.PermissionEnvelope{
				SourceCode: "read", Tests: "read", GitCommit: "deny",
				GitPush: "deny", Secrets: "deny", Production: "deny",
			},
			Mode: execution.ModeReadOnly,
		}},
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := runstate.BindTaskGraph(&run, []scheduler.Task{{
		ID: "research", AgentID: "researcher", Status: scheduler.StatusQueued,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := runstate.Transition(&run, runstate.StatusRunning, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := runstate.Transition(&run, runstate.StatusPaused, now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	runStore, err := runstate.NewStore(stateDir)
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
	appendPauseFixtureEvent(t, eventStore, run.ID, controlplane.EventRunStarted, "", "", map[string]any{"source": "test"}, now)
	appendPauseFixtureEvent(t, eventStore, run.ID, controlplane.EventRunPauseRequested, "", "", map[string]any{"source": "test"}, now.Add(time.Second))
	appendPauseFixtureEvent(t, eventStore, run.ID, controlplane.EventRunPaused, "", "", map[string]any{"source": "test"}, now.Add(2*time.Second))

	controlStore, err := runcontrol.NewStore(controlDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controlStore.RequestPause(run.ID, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}

	if err := runRunCancel([]string{
		run.ID,
		"--state-dir", stateDir,
		"--events-dir", eventsDir,
		"--control-dir", controlDir,
	}); err != nil {
		t.Fatal(err)
	}

	reloaded, err := runStore.Load(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Status != runstate.StatusCancelled {
		t.Fatalf("run=%#v", reloaded)
	}
	requested, err := controlStore.PauseRequested(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if requested {
		t.Fatal("pause marker remained after paused-run cancellation")
	}
	events, err := eventStore.List(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	state, err := controlplane.Project(events)
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != controlplane.RunCancelled || state.PauseRequested || state.CancelRequested {
		t.Fatalf("state=%#v", state)
	}
}
