package main

import (
	"context"
	"testing"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/controlplane"
	"github.com/FernandoVillegas27/ai-engineering-os/core/runcontrol"
	"github.com/FernandoVillegas27/ai-engineering-os/core/runstate"
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
	// The durable request may exist, but no invalid event stream may be created.
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
