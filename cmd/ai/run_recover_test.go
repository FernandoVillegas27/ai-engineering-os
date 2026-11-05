package main

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/controlplane"
	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/runcontrol"
	"github.com/FernandoVillegas27/ai-engineering-os/core/runstate"
	"github.com/FernandoVillegas27/ai-engineering-os/core/scheduler"
)

func staleRecoveryFixture(
	t *testing.T,
	stateDir, eventsDir, controlDir string,
	failed bool,
) runstate.Run {
	t.Helper()
	now := time.Now().UTC().Add(-time.Minute)
	units := []execution.WorkUnit{
		{
			ID: "research", AgentID: "researcher", Task: "research",
			Permissions: execution.PermissionEnvelope{
				SourceCode: "read", Tests: "read", GitCommit: "deny",
				GitPush: "deny", Secrets: "deny", Production: "deny",
			},
			Mode: execution.ModeReadOnly,
		},
		{
			ID: "architect", AgentID: "architect", Task: "design",
			Permissions: execution.PermissionEnvelope{
				SourceCode: "read", Tests: "read", GitCommit: "deny",
				GitPush: "deny", Secrets: "deny", Production: "deny",
			},
			Mode: execution.ModeReadOnly,
		},
	}
	run, err := runstate.New(runstate.NewRunRequest{
		Task: "recover feature", Intent: "research",
		ProjectID: "project", ProjectRoot: t.TempDir(),
		Runtime: runstate.RuntimeSelection{Provider: "dry-run", ModelPolicy: "auto"},
		WorkUnits: units,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := runstate.BindTaskGraph(&run, []scheduler.Task{
		{ID: "research", AgentID: "researcher"},
		{ID: "architect", AgentID: "architect", DependsOn: []string{"research"}},
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
	appendRecoveryEvent := func(typ controlplane.EventType, taskID, agentID string, data any, offset time.Duration) {
		t.Helper()
		event, err := controlplane.NewEvent(controlplane.NewEventRequest{
			RunID: run.ID, Type: typ, TaskID: taskID, AgentID: agentID, Data: data,
		}, now.Add(offset))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := eventStore.Append(context.Background(), event); err != nil {
			t.Fatal(err)
		}
	}
	appendRecoveryEvent(controlplane.EventRunStarted, "", "", map[string]any{"source": "test"}, 0)
	appendRecoveryEvent(controlplane.EventTaskCreated, "research", "", map[string]any{
		"depends_on": []string{}, "mutating": false, "scopes": []string{},
	}, time.Second)
	appendRecoveryEvent(controlplane.EventTaskAssigned, "research", "researcher", map[string]any{"source": "test"}, 2*time.Second)
	appendRecoveryEvent(controlplane.EventTaskCreated, "architect", "", map[string]any{
		"depends_on": []string{"research"}, "mutating": false, "scopes": []string{},
	}, 3*time.Second)
	appendRecoveryEvent(controlplane.EventTaskAssigned, "architect", "architect", map[string]any{"source": "test"}, 4*time.Second)
	appendRecoveryEvent(controlplane.EventTaskStarted, "research", "researcher", map[string]any{"source": "test"}, 5*time.Second)
	if failed {
		appendRecoveryEvent(controlplane.EventTaskFailed, "research", "researcher", map[string]any{"source": "test"}, 6*time.Second)
		appendRecoveryEvent(controlplane.EventTaskWaiting, "architect", "architect", map[string]any{"source": "test"}, 7*time.Second)
	} else {
		appendRecoveryEvent(controlplane.EventTaskCompleted, "research", "researcher", map[string]any{"source": "test"}, 6*time.Second)
		appendRecoveryEvent(controlplane.EventTaskStarted, "architect", "architect", map[string]any{"source": "test"}, 7*time.Second)
	}

	controlStore, err := runcontrol.NewStore(controlDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controlStore.AcquireLease(run.ID, now); err != nil {
		t.Fatal(err)
	}
	return run
}

func TestRunRecoverRequeuesInterruptedReadOnlyTask(t *testing.T) {
	base := t.TempDir()
	stateDir := base + "/runs"
	eventsDir := base + "/events"
	controlDir := base + "/control"
	run := staleRecoveryFixture(t, stateDir, eventsDir, controlDir, false)

	if err := runRunRecover([]string{
		run.ID,
		"--state-dir", stateDir,
		"--events-dir", eventsDir,
		"--control-dir", controlDir,
		"--stale-after", "5s",
	}); err != nil {
		t.Fatal(err)
	}

	runStore, err := runstate.NewStore(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := runStore.Load(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Status != runstate.StatusPaused {
		t.Fatalf("run=%#v", recovered)
	}
	statuses := map[string]runstate.WorkUnitStatus{}
	for _, unit := range recovered.WorkUnits {
		statuses[unit.ID] = unit.Status
	}
	if statuses["research"] != runstate.WorkUnitCompleted ||
		statuses["architect"] != runstate.WorkUnitQueued {
		t.Fatalf("statuses=%#v", statuses)
	}

	eventStore, err := controlplane.NewEventStore(eventsDir)
	if err != nil {
		t.Fatal(err)
	}
	events, err := eventStore.List(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	state, err := controlplane.Project(events)
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != controlplane.RunPaused ||
		state.Tasks["research"].Status != controlplane.TaskCompleted ||
		state.Tasks["architect"].Status != controlplane.TaskQueued {
		t.Fatalf("state=%#v", state)
	}

	controlStore, err := runcontrol.NewStore(controlDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controlStore.LoadLease(run.ID); !os.IsNotExist(err) {
		t.Fatalf("recovery lease was not released: %v", err)
	}
}

func TestRunRecoverConvertsObservedTaskFailureToFailedRun(t *testing.T) {
	base := t.TempDir()
	stateDir := base + "/runs"
	eventsDir := base + "/events"
	controlDir := base + "/control"
	run := staleRecoveryFixture(t, stateDir, eventsDir, controlDir, true)

	if err := runRunRecover([]string{
		run.ID,
		"--state-dir", stateDir,
		"--events-dir", eventsDir,
		"--control-dir", controlDir,
		"--stale-after", "5s",
	}); err != nil {
		t.Fatal(err)
	}
	store, err := runstate.NewStore(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := store.Load(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Status != runstate.StatusFailed {
		t.Fatalf("run=%#v", recovered)
	}
	if recovered.WorkUnits[0].Status != runstate.WorkUnitFailed {
		t.Fatalf("work units=%#v", recovered.WorkUnits)
	}
}

func TestRunRecoverRejectsLiveLeaseWithoutMutatingState(t *testing.T) {
	base := t.TempDir()
	stateDir := base + "/runs"
	eventsDir := base + "/events"
	controlDir := base + "/control"
	run := staleRecoveryFixture(t, stateDir, eventsDir, controlDir, false)

	controlStore, err := runcontrol.NewStore(controlDir)
	if err != nil {
		t.Fatal(err)
	}
	old, err := controlStore.LoadLease(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := controlStore.ReleaseLease(run.ID, old.OwnerID); err != nil {
		t.Fatal(err)
	}
	if _, err := controlStore.AcquireLease(run.ID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}

	err = runRunRecover([]string{
		run.ID,
		"--state-dir", stateDir,
		"--events-dir", eventsDir,
		"--control-dir", controlDir,
		"--stale-after", "30s",
	})
	if err == nil || !strings.Contains(err.Error(), "still live") {
		t.Fatalf("err=%#v", err)
	}
	store, err := runstate.NewStore(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	current, err := store.Load(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != runstate.StatusRunning {
		t.Fatalf("run mutated=%#v", current)
	}
}

func TestRunRecoverRejectsMutatingRunWithoutCheckpoint(t *testing.T) {
	base := t.TempDir()
	stateDir := base + "/runs"
	eventsDir := base + "/events"
	controlDir := base + "/control"
	now := time.Now().UTC().Add(-time.Minute)
	unit := execution.WorkUnit{
		ID: "backend", AgentID: "backend-engineer", Task: "implement",
		Permissions: execution.PermissionEnvelope{
			SourceCode: "read", Tests: "read", GitCommit: "deny",
			GitPush: "deny", Secrets: "deny", Production: "deny",
		},
		Mutation: execution.MutationCapability{SourceCode: true, Tests: true},
		Mode: execution.ModeReadOnly,
	}
	run, err := runstate.New(runstate.NewRunRequest{
		Task: "implement", Intent: "feature", ProjectID: "project", ProjectRoot: t.TempDir(),
		Runtime: runstate.RuntimeSelection{Provider: "cursor", ModelPolicy: "auto"},
		WorkUnits: []execution.WorkUnit{unit},
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := runstate.BindTaskGraph(&run, []scheduler.Task{{
		ID: "backend", AgentID: "backend-engineer", Mutating: true,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := runstate.BindMutationKinds(&run, []execution.WorkUnit{unit}); err != nil {
		t.Fatal(err)
	}
	if err := runstate.Transition(&run, runstate.StatusRunning, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	store, err := runstate.NewStore(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(run); err != nil {
		t.Fatal(err)
	}
	eventStore, err := controlplane.NewEventStore(eventsDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		typ controlplane.EventType
		task, agent string
		data any
	}{
		{controlplane.EventRunStarted, "", "", map[string]any{"source": "test"}},
		{controlplane.EventTaskCreated, "backend", "", map[string]any{"depends_on": []string{}, "mutating": true, "scopes": []string{}}},
		{controlplane.EventTaskAssigned, "backend", "backend-engineer", map[string]any{"source": "test"}},
		{controlplane.EventTaskStarted, "backend", "backend-engineer", map[string]any{"source": "test"}},
	} {
		event, err := controlplane.NewEvent(controlplane.NewEventRequest{
			RunID: run.ID, Type: item.typ, TaskID: item.task, AgentID: item.agent, Data: item.data,
		}, now)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := eventStore.Append(context.Background(), event); err != nil {
			t.Fatal(err)
		}
	}
	controlStore, err := runcontrol.NewStore(controlDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controlStore.AcquireLease(run.ID, now); err != nil {
		t.Fatal(err)
	}
	if err := runRunRecover([]string{
		run.ID, "--state-dir", stateDir, "--events-dir", eventsDir,
		"--control-dir", controlDir, "--stale-after", "5s",
	}); err == nil {
		t.Fatal("expected mutation checkpoint recovery rejection")
	}
}

func TestParseRunRecoverArgs(t *testing.T) {
	options, err := parseRunRecoverArgs([]string{
		"run-1", "--stale-after", "45s", "--json",
	})
	if err != nil {
		t.Fatal(err)
	}
	if options.runID != "run-1" || options.staleAfter != 45*time.Second || !options.json {
		t.Fatalf("options=%#v", options)
	}
	if _, err := parseRunRecoverArgs([]string{"run-1", "--stale-after", "1s"}); err == nil {
		t.Fatal("expected too-small stale threshold rejection")
	}
}
