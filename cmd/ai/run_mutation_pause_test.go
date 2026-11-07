package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/controlplane"
	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/runcheckpoint"
	"github.com/FernandoVillegas27/ai-engineering-os/core/runcontrol"
	"github.com/FernandoVillegas27/ai-engineering-os/core/runstate"
)

func mutationCheckpointFixtureRun(t *testing.T) runstate.Run {
	t.Helper()
	now := time.Now().UTC().Add(-time.Minute)
	run, err := runstate.New(runstate.NewRunRequest{
		Task: "implement",
		Intent: "feature",
		ProjectID: "project",
		ProjectRoot: t.TempDir(),
		Runtime: runstate.RuntimeSelection{
			Provider: "cursor",
			ModelPolicy: "auto",
		},
		WorkUnits: []execution.WorkUnit{
			{
				ID: "architect", AgentID: "architect", Task: "implement",
				Permissions: execution.PermissionEnvelope{SourceCode: "read", Tests: "read", GitCommit: "deny", GitPush: "deny", Secrets: "deny", Production: "deny"},
				Mode: execution.ModeReadOnly,
			},
			{
				ID: "backend", AgentID: "backend-engineer", Task: "implement",
				Tools: []string{"filesystem"},
				ToolOperations: map[string][]string{"filesystem": {"read-files", "write-files"}},
				Permissions: execution.PermissionEnvelope{SourceCode: "read", Tests: "read", GitCommit: "deny", GitPush: "deny", Secrets: "deny", Production: "deny"},
				Mutation: execution.MutationCapability{SourceCode: true, Tests: true},
				Mode: execution.ModeReadOnly,
			},
			{
				ID: "qa", AgentID: "qa-engineer", Task: "implement",
				Tools: []string{"filesystem"},
				ToolOperations: map[string][]string{"filesystem": {"read-files", "write-files"}},
				Permissions: execution.PermissionEnvelope{SourceCode: "read", Tests: "read", GitCommit: "deny", GitPush: "deny", Secrets: "deny", Production: "deny"},
				Mutation: execution.MutationCapability{Tests: true},
				Mode: execution.ModeReadOnly,
			},
		},
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	run.WorkUnits[0].Status = runstate.WorkUnitQueued
	run.WorkUnits[1].Status = runstate.WorkUnitQueued
	run.WorkUnits[1].Mutating = true
	run.WorkUnits[1].MutationKind = "source"
	run.WorkUnits[2].Status = runstate.WorkUnitQueued
	run.WorkUnits[2].Mutating = true
	run.WorkUnits[2].MutationKind = "tests-only"
	if err := runstate.Transition(&run, runstate.StatusRunning, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	return run
}

func TestPersistMutationCheckpointStateTracksSourceAndFinalProgress(t *testing.T) {
	run := mutationCheckpointFixtureRun(t)
	store, err := runstate.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := persistMutationCheckpointState(&run, store, runcheckpoint.PhaseSourceIntegrated); err != nil {
		t.Fatal(err)
	}
	statuses := map[string]runstate.WorkUnitStatus{}
	for _, unit := range run.WorkUnits {
		statuses[unit.ID] = unit.Status
	}
	if statuses["architect"] != runstate.WorkUnitCompleted ||
		statuses["backend"] != runstate.WorkUnitCompleted ||
		statuses["qa"] != runstate.WorkUnitQueued {
		t.Fatalf("source statuses=%#v", statuses)
	}
	if err := persistMutationCheckpointState(&run, store, runcheckpoint.PhaseFinalIntegrated); err != nil {
		t.Fatal(err)
	}
	for _, unit := range run.WorkUnits {
		if unit.Status != runstate.WorkUnitCompleted {
			t.Fatalf("final work units=%#v", run.WorkUnits)
		}
	}
}

func TestPauseMutationAtCheckpointTransitionsRunAndProjectsPhase(t *testing.T) {
	run := mutationCheckpointFixtureRun(t)
	runStore, err := runstate.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := runStore.Save(run); err != nil {
		t.Fatal(err)
	}
	eventStore, err := controlplane.NewEventStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	started, err := controlplane.NewEvent(controlplane.NewEventRequest{
		RunID: run.ID, Type: controlplane.EventRunStarted, Data: map[string]any{"source": "test"},
	}, time.Now().UTC().Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := eventStore.Append(context.Background(), started); err != nil {
		t.Fatal(err)
	}
	controlStore, err := runcontrol.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controlStore.RequestPause(run.ID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}

	paused, err := pauseMutationAtCheckpoint(
		context.Background(),
		&run,
		runStore,
		eventStore,
		controlStore,
		runcheckpoint.PhaseSourceIntegrated,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !paused || run.Status != runstate.StatusPaused {
		t.Fatalf("run=%#v paused=%t", run, paused)
	}
	events, err := eventStore.List(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	state, err := controlplane.Project(events)
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != controlplane.RunPaused {
		t.Fatalf("state=%#v", state)
	}
	if len(run.WorkUnits) != 3 ||
		run.WorkUnits[0].Status != runstate.WorkUnitCompleted ||
		run.WorkUnits[1].Status != runstate.WorkUnitCompleted ||
		run.WorkUnits[2].Status != runstate.WorkUnitQueued {
		t.Fatalf("work units=%#v", run.WorkUnits)
	}
}

func TestReconstructMutationUnitsUsesDurableMutationKind(t *testing.T) {
	run := mutationCheckpointFixtureRun(t)
	testsOnly, err := reconstructMutationUnits(run, "tests-only")
	if err != nil {
		t.Fatal(err)
	}
	if len(testsOnly) != 1 || testsOnly[0].ID != "qa" ||
		!testsOnly[0].Mutation.Tests || testsOnly[0].Mutation.SourceCode {
		t.Fatalf("testsOnly=%#v", testsOnly)
	}
	if got := testsOnly[0].ToolOperations["filesystem"]; len(got) != 2 {
		t.Fatalf("testsOnly tool operations=%#v", testsOnly[0].ToolOperations)
	}
	source, err := reconstructMutationUnits(run, "source")
	if err != nil {
		t.Fatal(err)
	}
	if len(source) != 1 || source[0].ID != "backend" ||
		!source[0].Mutation.SourceCode {
		t.Fatalf("source=%#v", source)
	}
}

func TestReconstructMutationUnitsRejectsLegacyMutatingUnitWithoutKind(t *testing.T) {
	run := mutationCheckpointFixtureRun(t)
	for i := range run.WorkUnits {
		if run.WorkUnits[i].ID == "qa" {
			run.WorkUnits[i].MutationKind = ""
		}
	}
	if _, err := reconstructMutationUnits(run, "tests-only"); err == nil {
		t.Fatal("expected missing mutation kind rejection")
	}
}


func TestRunResumeRoutesMutatingPausedRunThroughCheckpointRestore(t *testing.T) {
	base := t.TempDir()
	stateDir := base + "/runs"
	eventsDir := base + "/events"
	checkpointDir := base + "/checkpoints"
	now := time.Now().UTC().Add(-time.Minute)

	run := mutationCheckpointFixtureRun(t)
	run.ProjectCodeTrusted = true
	run.ProjectRoot = base + "/project"
	if err := os.MkdirAll(run.ProjectRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(run.ProjectRoot, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runstate.Transition(&run, runstate.StatusPaused, now.Add(2*time.Second)); err != nil {
		// fixture timestamps may be newer than this local now; use current durable time.
		at := time.Now().UTC()
		if at.Before(run.UpdatedAt) {
			at = run.UpdatedAt
		}
		if retryErr := runstate.Transition(&run, runstate.StatusPaused, at); retryErr != nil {
			t.Fatal(retryErr)
		}
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
	append := func(typ controlplane.EventType, data any) {
		t.Helper()
		event, err := controlplane.NewEvent(controlplane.NewEventRequest{
			RunID: run.ID, Type: typ, Data: data,
		}, time.Now().UTC())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := eventStore.Append(context.Background(), event); err != nil {
			t.Fatal(err)
		}
	}
	append(controlplane.EventRunStarted, map[string]any{"source": "test"})
	append(controlplane.EventCheckpointCreated, map[string]any{"phase": "final-integrated"})
	append(controlplane.EventRunPaused, map[string]any{"phase": "final-integrated"})

	err = runRunResume([]string{
		run.ID,
		"--state-dir", stateDir,
		"--events-dir", eventsDir,
		"--checkpoint-dir", checkpointDir,
	})
	if err == nil || !strings.Contains(err.Error(), "restore mutation checkpoint") {
		t.Fatalf("err=%#v", err)
	}
}


func TestRunResumeMutatingLegacyRunRequiresExplicitTrust(t *testing.T) {
	base := t.TempDir()
	stateDir := base + "/runs"
	run := mutationCheckpointFixtureRun(t)
	run.ProjectCodeTrusted = false
	run.ProjectRoot = base + "/project"
	if err := os.MkdirAll(run.ProjectRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC()
	if at.Before(run.UpdatedAt) {
		at = run.UpdatedAt
	}
	if err := runstate.Transition(&run, runstate.StatusPaused, at); err != nil {
		t.Fatal(err)
	}
	store, err := runstate.NewStore(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(run); err != nil {
		t.Fatal(err)
	}

	err = runRunResume([]string{run.ID, "--state-dir", stateDir})
	if err == nil || !strings.Contains(err.Error(), "no durable project-code trust evidence") {
		t.Fatalf("err=%#v", err)
	}

	// Explicit trust is persisted before downstream resume validation proceeds.
	err = runRunResume([]string{run.ID, "--state-dir", stateDir, "--trust-project-code"})
	if err == nil {
		t.Fatal("expected later resume validation error because no event stream fixture exists")
	}
	loaded, loadErr := store.Load(run.ID)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if !loaded.ProjectCodeTrusted {
		t.Fatalf("explicit resume trust was not persisted: %#v", loaded)
	}
}
