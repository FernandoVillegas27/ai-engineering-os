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

func appendPauseFixtureEvent(
	t *testing.T,
	store *controlplane.EventStore,
	runID string,
	typ controlplane.EventType,
	taskID string,
	agentID string,
	data any,
	at time.Time,
) {
	t.Helper()
	value, err := controlplane.NewEvent(controlplane.NewEventRequest{
		RunID:  runID,
		Type:   typ,
		TaskID: taskID,
		AgentID: agentID,
		Data:   data,
	}, at)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Append(context.Background(), value); err != nil {
		t.Fatal(err)
	}
}

func TestRunPausePersistsRequestAndProjectsPendingPause(t *testing.T) {
	base := t.TempDir()
	stateDir := base + "/runs"
	eventsDir := base + "/events"
	controlDir := base + "/control"
	now := time.Now().UTC().Add(-time.Minute)

	run, err := runstate.New(runstate.NewRunRequest{
		Task:        "analyze",
		Intent:      "research",
		ProjectID:   "project",
		ProjectRoot: base,
		Runtime:     runstate.RuntimeSelection{Provider: "dry-run", ModelPolicy: "auto"},
		WorkUnits: []execution.WorkUnit{{
			ID: "research", AgentID: "researcher", Task: "analyze",
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
	if err := runstate.BindTaskGraph(&run, []scheduler.Task{
		{ID: "research", AgentID: "researcher", Status: scheduler.StatusQueued},
	}); err != nil {
		t.Fatal(err)
	}
	if err := runstate.Transition(&run, runstate.StatusRunning, now.Add(time.Second)); err != nil {
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
	appendPauseFixtureEvent(t, eventStore, run.ID, controlplane.EventTaskCreated, "research", "", map[string]any{
		"depends_on": []string{}, "mutating": false, "scopes": []string{},
	}, now.Add(time.Second))
	appendPauseFixtureEvent(t, eventStore, run.ID, controlplane.EventTaskAssigned, "research", "researcher", map[string]any{"source": "test"}, now.Add(2*time.Second))

	if err := runRunPause([]string{
		run.ID,
		"--state-dir", stateDir,
		"--events-dir", eventsDir,
		"--control-dir", controlDir,
	}); err != nil {
		t.Fatal(err)
	}

	controlStore, err := runcontrol.NewStore(controlDir)
	if err != nil {
		t.Fatal(err)
	}
	requested, err := controlStore.PauseRequested(run.ID)
	if err != nil || !requested {
		t.Fatalf("pause requested=%t err=%v", requested, err)
	}

	events, err := eventStore.List(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	state, err := controlplane.Project(events)
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != controlplane.RunRunning || !state.PauseRequested {
		t.Fatalf("state=%#v", state)
	}
}

func TestRunResumeContinuesSamePausedRunWithoutDuplicatingTasks(t *testing.T) {
	base := t.TempDir()
	stateDir := base + "/runs"
	eventsDir := base + "/events"
	artifactsDir := base + "/artifacts"
	messagesDir := base + "/messages"
	controlDir := base + "/control"
	now := time.Now().UTC().Add(-time.Minute)

	units := []execution.WorkUnit{
		{
			ID: "research", AgentID: "researcher", Task: "analyze",
			Permissions: execution.PermissionEnvelope{
				SourceCode: "read", Tests: "read", GitCommit: "deny",
				GitPush: "deny", Secrets: "deny", Production: "deny",
			},
			Mode: execution.ModeReadOnly,
		},
		{
			ID: "architect", AgentID: "architect", Task: "analyze",
			Permissions: execution.PermissionEnvelope{
				SourceCode: "read", Tests: "read", GitCommit: "deny",
				GitPush: "deny", Secrets: "deny", Production: "deny",
			},
			Mode: execution.ModeReadOnly,
		},
	}
	run, err := runstate.New(runstate.NewRunRequest{
		Task:        "analyze",
		Intent:      "research",
		ProjectID:   "project",
		ProjectRoot: base,
		Runtime:     runstate.RuntimeSelection{Provider: "dry-run", ModelPolicy: "auto"},
		WorkUnits:   units,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := runstate.BindTaskGraph(&run, []scheduler.Task{
		{ID: "research", AgentID: "researcher", Status: scheduler.StatusCompleted},
		{ID: "architect", AgentID: "architect", DependsOn: []string{"research"}, Status: scheduler.StatusQueued},
	}); err != nil {
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
	appendPauseFixtureEvent(t, eventStore, run.ID, controlplane.EventTaskCreated, "research", "", map[string]any{
		"depends_on": []string{}, "mutating": false, "scopes": []string{},
	}, now.Add(time.Second))
	appendPauseFixtureEvent(t, eventStore, run.ID, controlplane.EventTaskAssigned, "research", "researcher", map[string]any{"source": "test"}, now.Add(2*time.Second))
	appendPauseFixtureEvent(t, eventStore, run.ID, controlplane.EventTaskCreated, "architect", "", map[string]any{
		"depends_on": []string{"research"}, "mutating": false, "scopes": []string{},
	}, now.Add(3*time.Second))
	appendPauseFixtureEvent(t, eventStore, run.ID, controlplane.EventTaskAssigned, "architect", "architect", map[string]any{"source": "test"}, now.Add(4*time.Second))
	appendPauseFixtureEvent(t, eventStore, run.ID, controlplane.EventTaskStarted, "research", "researcher", map[string]any{"source": "test"}, now.Add(5*time.Second))
	appendPauseFixtureEvent(t, eventStore, run.ID, controlplane.EventTaskCompleted, "research", "researcher", map[string]any{"source": "test"}, now.Add(6*time.Second))
	appendPauseFixtureEvent(t, eventStore, run.ID, controlplane.EventRunPauseRequested, "", "", map[string]any{"source": "test"}, now.Add(7*time.Second))
	appendPauseFixtureEvent(t, eventStore, run.ID, controlplane.EventRunPaused, "", "", map[string]any{"source": "test"}, now.Add(8*time.Second))

	controlStore, err := runcontrol.NewStore(controlDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controlStore.RequestPause(run.ID, now.Add(7*time.Second)); err != nil {
		t.Fatal(err)
	}

	if err := runRunResume([]string{
		run.ID,
		"--state-dir", stateDir,
		"--events-dir", eventsDir,
		"--artifacts-dir", artifactsDir,
		"--messages-dir", messagesDir,
		"--control-dir", controlDir,
		"--max-concurrency", "1",
	}); err != nil {
		t.Fatal(err)
	}

	reloaded, err := runStore.Load(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.ID != run.ID || reloaded.Status != runstate.StatusCompleted {
		t.Fatalf("reloaded=%#v", reloaded)
	}
	for _, unit := range reloaded.WorkUnits {
		if unit.Status != runstate.WorkUnitCompleted {
			t.Fatalf("work units=%#v", reloaded.WorkUnits)
		}
	}
	requested, err := controlStore.PauseRequested(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if requested {
		t.Fatal("pause marker was not cleared")
	}

	events, err := eventStore.List(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	createdCount := map[string]int{}
	resumedCount := 0
	for _, event := range events {
		if event.Type == controlplane.EventTaskCreated {
			createdCount[event.TaskID]++
		}
		if event.Type == controlplane.EventRunResumed {
			resumedCount++
		}
	}
	if createdCount["research"] != 1 || createdCount["architect"] != 1 || resumedCount != 1 {
		t.Fatalf("events=%#v", events)
	}
	state, err := controlplane.Project(events)
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != controlplane.RunCompleted || state.Progress.Completed != 2 {
		t.Fatalf("state=%#v", state)
	}
}

func TestRunPauseAndResumeRejectMutatingRuns(t *testing.T) {
	base := t.TempDir()
	stateDir := base + "/runs"
	now := time.Now().UTC().Add(-time.Minute)
	makeRun := func(status runstate.Status) runstate.Run {
		t.Helper()
		run, err := runstate.New(runstate.NewRunRequest{
			Task: "implement", Intent: "feature", ProjectID: "project", ProjectRoot: base,
			Runtime: runstate.RuntimeSelection{Provider: "dry-run", ModelPolicy: "auto"},
			WorkUnits: []execution.WorkUnit{{
				ID: "backend", AgentID: "backend-engineer", Task: "implement",
				Permissions: execution.PermissionEnvelope{
					SourceCode: "read", Tests: "read", GitCommit: "deny",
					GitPush: "deny", Secrets: "deny", Production: "deny",
				},
				Mutation: execution.MutationCapability{SourceCode: true, Tests: true},
				Mode: execution.ModeReadOnly,
			}},
		}, now)
		if err != nil {
			t.Fatal(err)
		}
		if err := runstate.BindTaskGraph(&run, []scheduler.Task{{
			ID: "backend", AgentID: "backend-engineer", Mutating: true, Status: scheduler.StatusQueued,
		}}); err != nil {
			t.Fatal(err)
		}
		if err := runstate.Transition(&run, runstate.StatusRunning, now.Add(time.Second)); err != nil {
			t.Fatal(err)
		}
		if status == runstate.StatusPaused {
			if err := runstate.Transition(&run, runstate.StatusPaused, now.Add(2*time.Second)); err != nil {
				t.Fatal(err)
			}
		}
		return run
	}
	store, err := runstate.NewStore(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	running := makeRun(runstate.StatusRunning)
	if err := store.Save(running); err != nil {
		t.Fatal(err)
	}
	if err := runRunPause([]string{running.ID, "--state-dir", stateDir}); err == nil {
		t.Fatal("expected mutating pause rejection")
	}

	paused := makeRun(runstate.StatusPaused)
	if err := store.Save(paused); err != nil {
		t.Fatal(err)
	}
	if err := runRunResume([]string{paused.ID, "--state-dir", stateDir}); err == nil {
		t.Fatal("expected mutating resume rejection")
	}
}

func TestParseRunPauseResumeArgs(t *testing.T) {
	pause, err := parseRunPauseArgs([]string{"run-1", "--json"})
	if err != nil || pause.runID != "run-1" || !pause.json {
		t.Fatalf("pause=%#v err=%v", pause, err)
	}
	resume, err := parseRunResumeArgs([]string{"run-1", "--max-concurrency", "4", "--json"})
	if err != nil {
		t.Fatal(err)
	}
	if resume.runID != "run-1" || resume.maxConcurrency != 4 || !resume.json {
		t.Fatalf("resume=%#v", resume)
	}
}


func TestParseRunResumeArgsProjectCodeTrust(t *testing.T) {
	options, err := parseRunResumeArgs([]string{
		"run-legacy",
		"--trust-project-code",
		"--max-concurrency", "3",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !options.trustProjectCode || options.runID != "run-legacy" || options.maxConcurrency != 3 {
		t.Fatalf("options=%#v", options)
	}
}
