package controlplane

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestProjectFromStateMatchesFullReplay(t *testing.T) {
	store, err := NewEventStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	snapshots, err := NewSnapshotStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runID := "run-project-snapshot"
	appendEvent(t, store, runID, EventRunStarted, "", "")
	appendEvent(t, store, runID, EventTaskStarted, "task-1", "architect")
	record, err := snapshots.Create(context.Background(), store, runID, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}

	appendEvent(t, store, runID, EventVerificationStarted, "", "")
	appendEvent(t, store, runID, EventVerificationPassed, "", "")
	appendEvent(t, store, runID, EventTaskCompleted, "task-1", "architect")
	appendEvent(t, store, runID, EventRunCompleted, "", "")

	all, err := store.List(runID)
	if err != nil {
		t.Fatal(err)
	}
	full, err := Project(all)
	if err != nil {
		t.Fatal(err)
	}
	suffix := all[record.ThroughSequence:]
	continued, err := ProjectFromState(record.State, suffix)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(full, continued) {
		t.Fatalf("full=%#v continued=%#v", full, continued)
	}

	verified, used, err := snapshots.ProjectVerified(context.Background(), store, runID)
	if err != nil {
		t.Fatal(err)
	}
	if !used || !reflect.DeepEqual(full, verified) {
		t.Fatalf("used=%t full=%#v verified=%#v", used, full, verified)
	}
}

func TestProjectVerifiedFallsBackToFullReplayWithoutSnapshot(t *testing.T) {
	store, err := NewEventStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	snapshots, err := NewSnapshotStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runID := "run-no-snapshot"
	appendEvent(t, store, runID, EventRunStarted, "", "")
	appendEvent(t, store, runID, EventTaskStarted, "task-1", "architect")

	state, used, err := snapshots.ProjectVerified(context.Background(), store, runID)
	if err != nil {
		t.Fatal(err)
	}
	if used || state.LastSequence != 2 || state.Status != RunRunning {
		t.Fatalf("used=%t state=%#v", used, state)
	}
}

func TestProjectVerifiedRejectsHistoricalPrefixMutation(t *testing.T) {
	eventRoot := t.TempDir()
	store, err := NewEventStore(eventRoot)
	if err != nil {
		t.Fatal(err)
	}
	snapshots, err := NewSnapshotStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runID := "run-prefix-mutation"
	appendEvent(t, store, runID, EventRunStarted, "", "")
	appendEvent(t, store, runID, EventTaskStarted, "task-1", "architect")
	if _, err := snapshots.Create(context.Background(), store, runID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	appendEvent(t, store, runID, EventTaskCompleted, "task-1", "architect")

	runDir := filepath.Join(eventRoot, runID)
	entries, err := os.ReadDir(runDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "00000000000000000001-") {
			data, err := os.ReadFile(filepath.Join(runDir, entry.Name()))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(runDir, entry.Name()), append(data, ' '), 0o600); err != nil {
				t.Fatal(err)
			}
			break
		}
	}
	if _, _, err := snapshots.ProjectVerified(context.Background(), store, runID); err == nil ||
		!strings.Contains(err.Error(), "prefix digest mismatch") {
		t.Fatalf("err=%#v", err)
	}
}

func TestProjectVerifiedRejectsCorruptSuffix(t *testing.T) {
	eventRoot := t.TempDir()
	store, err := NewEventStore(eventRoot)
	if err != nil {
		t.Fatal(err)
	}
	snapshots, err := NewSnapshotStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runID := "run-corrupt-suffix"
	appendEvent(t, store, runID, EventRunStarted, "", "")
	if _, err := snapshots.Create(context.Background(), store, runID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	appendEvent(t, store, runID, EventTaskStarted, "task-1", "architect")

	runDir := filepath.Join(eventRoot, runID)
	entries, err := os.ReadDir(runDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "00000000000000000002-") {
			if err := os.WriteFile(filepath.Join(runDir, entry.Name()), []byte("{bad json"), 0o600); err != nil {
				t.Fatal(err)
			}
			break
		}
	}
	if _, _, err := snapshots.ProjectVerified(context.Background(), store, runID); err == nil {
		t.Fatal("expected corrupt suffix rejection")
	}
}

func TestProjectFromStateDoesNotMutateBaseSnapshotState(t *testing.T) {
	store, err := NewEventStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	snapshots, err := NewSnapshotStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runID := "run-base-immutable"
	appendEvent(t, store, runID, EventRunStarted, "", "")
	appendEvent(t, store, runID, EventTaskStarted, "task-1", "architect")
	record, err := snapshots.Create(context.Background(), store, runID, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	before := cloneProjectionState(record.State)

	completed := appendEvent(t, store, runID, EventTaskCompleted, "task-1", "architect")
	if _, err := ProjectFromState(record.State, []Event{completed}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, record.State) {
		t.Fatalf("base state mutated: before=%#v after=%#v", before, record.State)
	}
}

func TestProjectFromStateRejectsGapMixedRunAndRunStarted(t *testing.T) {
	base := State{
		RunID:        "run-a",
		Status:       RunRunning,
		LastSequence: 2,
		Tasks:        map[string]TaskState{},
		Agents:       map[string]AgentState{},
	}
	base.recompute()

	value, err := NewEvent(NewEventRequest{
		RunID: "run-a", Type: EventArtifactCreated, Data: map[string]bool{"ok": true},
	}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	value.Sequence = 4
	if _, err := ProjectFromState(base, []Event{value}); err == nil {
		t.Fatal("expected continuation gap rejection")
	}

	value.Sequence = 3
	value.RunID = "run-b"
	if _, err := ProjectFromState(base, []Event{value}); err == nil {
		t.Fatal("expected mixed run rejection")
	}

	started, err := NewEvent(NewEventRequest{
		RunID: "run-a", Type: EventRunStarted, Data: map[string]bool{"ok": true},
	}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	started.Sequence = 3
	if _, err := ProjectFromState(base, []Event{started}); err == nil {
		t.Fatal("expected run.started continuation rejection")
	}
}


func TestProjectFromStateDeepClonesToolOperationAuthority(t *testing.T) {
	base := State{
		RunID: "run-tool-authority",
		Status: RunRunning,
		LastSequence: 2,
		Tasks: map[string]TaskState{
			"task-1": {
				ID: "task-1",
				AgentID: "researcher",
				Status: TaskQueued,
				ToolOperations: map[string][]string{
					"github": {"read-repository", "read-issues"},
				},
				LastSequence: 2,
			},
		},
		Agents: map[string]AgentState{},
	}
	base.recompute()

	continued, err := ProjectFromState(base, nil)
	if err != nil {
		t.Fatal(err)
	}
	task := continued.Tasks["task-1"]
	task.ToolOperations["github"][0] = "create-commit"
	continued.Tasks["task-1"] = task

	got := base.Tasks["task-1"].ToolOperations["github"]
	if len(got) != 2 || got[0] != "read-repository" {
		t.Fatalf("base tool authority was mutated through clone: %#v", got)
	}
}
