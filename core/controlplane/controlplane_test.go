package controlplane

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/coordinator"
	"github.com/FernandoVillegas27/ai-engineering-os/core/scheduler"
)

func appendEvent(t *testing.T, store *EventStore, runID string, typ EventType, taskID, agentID string) Event {
	t.Helper()
	value, err := NewEvent(NewEventRequest{
		RunID:   runID,
		Type:    typ,
		TaskID:  taskID,
		AgentID: agentID,
		Data:    map[string]string{"test": "true"},
	}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	value, err = store.Append(context.Background(), value)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestEventStoreAssignsContiguousPerRunSequence(t *testing.T) {
	store, err := NewEventStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	first := appendEvent(t, store, "run-1", EventRunStarted, "", "")
	second := appendEvent(t, store, "run-1", EventTaskStarted, "task-a", "architect")
	third := appendEvent(t, store, "run-1", EventTaskCompleted, "task-a", "architect")
	if first.Sequence != 1 || second.Sequence != 2 || third.Sequence != 3 {
		t.Fatalf("sequences = %d,%d,%d", first.Sequence, second.Sequence, third.Sequence)
	}
	events, err := store.List("run-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 || events[1].Type != EventTaskStarted {
		t.Fatalf("events = %#v", events)
	}
}

func TestEventStoreRejectsTraversalAndSymlinkRunDirectory(t *testing.T) {
	root := t.TempDir()
	store, err := NewEventStore(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.List("../outside"); err == nil {
		t.Fatal("expected unsafe run id rejection")
	}

	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(root, "run-link")); err != nil {
		if os.IsPermission(err) {
			t.Skip("symlink creation is not permitted")
		}
		t.Fatal(err)
	}
	if _, err := store.List("run-link"); err == nil {
		t.Fatal("expected symlink run directory rejection")
	}
}

func TestProjectorBuildsAgentTaskQualityAndControlState(t *testing.T) {
	store, err := NewEventStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runID := "run-control"
	appendEvent(t, store, runID, EventRunStarted, "", "")
	appendEvent(t, store, runID, EventTaskStarted, "backend", "backend-engineer")
	appendEvent(t, store, runID, EventVerificationStarted, "", "")
	appendEvent(t, store, runID, EventBaselineCompared, "", "")
	appendEvent(t, store, runID, EventVerificationPassed, "", "")
	appendEvent(t, store, runID, EventReviewStarted, "", "")
	appendEvent(t, store, runID, EventReviewApproved, "", "")
	appendEvent(t, store, runID, EventApprovalRequested, "", "")
	appendEvent(t, store, runID, EventAgentMessageSent, "", "")
	appendEvent(t, store, runID, EventArtifactCreated, "", "")
	appendEvent(t, store, runID, EventTaskCompleted, "backend", "backend-engineer")
	appendEvent(t, store, runID, EventApprovalGranted, "", "")
	appendEvent(t, store, runID, EventRunCompleted, "", "")

	events, err := store.List(runID)
	if err != nil {
		t.Fatal(err)
	}
	state, err := Project(events)
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != RunCompleted || state.LastSequence != 13 {
		t.Fatalf("state = %#v", state)
	}
	if state.Progress.Total != 1 || state.Progress.Completed != 1 {
		t.Fatalf("progress = %#v", state.Progress)
	}
	agent := state.Agents["backend-engineer"]
	if agent.Status != AgentCompleted || agent.TaskCount != 1 {
		t.Fatalf("agent = %#v", agent)
	}
	if state.Quality.Verification != "passed" || state.Quality.Review != "approved" || state.Quality.BaselineComparisons != 1 {
		t.Fatalf("quality = %#v", state.Quality)
	}
	if state.MessageCount != 1 || state.ArtifactCount != 1 || state.PendingApprovals != 0 {
		t.Fatalf("state counters = %#v", state)
	}
}

func TestProjectorRejectsMixedRunsGapsAndPostTerminalMutation(t *testing.T) {
	first, err := NewEvent(NewEventRequest{RunID: "run-a", Type: EventRunStarted, Data: map[string]bool{}}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	first.Sequence = 1
	second, err := NewEvent(NewEventRequest{RunID: "run-b", Type: EventRunCompleted, Data: map[string]bool{}}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	second.Sequence = 2
	if _, err := Project([]Event{first, second}); err == nil {
		t.Fatal("expected mixed run rejection")
	}

	second.RunID = "run-a"
	second.Sequence = 3
	if _, err := Project([]Event{first, second}); err == nil {
		t.Fatal("expected sequence gap rejection")
	}

	second.Sequence = 2
	third, err := NewEvent(NewEventRequest{
		RunID:   "run-a",
		Type:    EventTaskStarted,
		TaskID:  "late",
		AgentID: "backend-engineer",
		Data:    map[string]bool{},
	}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	third.Sequence = 3
	if _, err := Project([]Event{first, second, third}); err == nil {
		t.Fatal("expected post-terminal task mutation rejection")
	}
}

type cpWorker func(context.Context, scheduler.Task) (coordinator.WorkResult, error)

func (f cpWorker) Execute(ctx context.Context, task scheduler.Task) (coordinator.WorkResult, error) {
	return f(ctx, task)
}

func TestCoordinatorSinkMakesExecutionReconstructable(t *testing.T) {
	store, err := NewEventStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	graph, err := scheduler.New([]scheduler.Task{
		{ID: "architect", AgentID: "architect"},
		{ID: "backend", AgentID: "backend-engineer", DependsOn: []string{"architect"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 4, 9, 0, 0, 0, time.UTC)
	sink := CoordinatorSink{
		RunID: "run-e2e",
		Store: store,
		Now:   func() time.Time { return now },
	}
	result, err := (coordinator.Coordinator{
		Config: coordinator.Config{MaxConcurrency: 2},
		Workers: map[string]coordinator.Worker{
			"architect": cpWorker(func(context.Context, scheduler.Task) (coordinator.WorkResult, error) {
				return coordinator.WorkResult{Summary: "architecture ready"}, nil
			}),
			"backend-engineer": cpWorker(func(context.Context, scheduler.Task) (coordinator.WorkResult, error) {
				return coordinator.WorkResult{Summary: "backend ready"}, nil
			}),
		},
		Events: sink,
	}).Run(context.Background(), graph)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Completed {
		t.Fatalf("coordinator result = %#v", result)
	}

	events, err := store.List("run-e2e")
	if err != nil {
		t.Fatal(err)
	}
	state, err := Project(events)
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != RunCompleted || state.Progress.Completed != 2 {
		t.Fatalf("state = %#v", state)
	}
	if state.Agents["architect"].Status != AgentCompleted || state.Agents["backend-engineer"].Status != AgentCompleted {
		t.Fatalf("agents = %#v", state.Agents)
	}
}

func TestAppendLockHonorsContextCancellation(t *testing.T) {
	root := t.TempDir()
	store, err := NewEventStore(root)
	if err != nil {
		t.Fatal(err)
	}
	runDir, err := store.ensureRunDir("run-lock")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, ".append.lock"), []byte("held"), 0o600); err != nil {
		t.Fatal(err)
	}
	value, err := NewEvent(NewEventRequest{
		RunID: "run-lock",
		Type:  EventRunStarted,
		Data:  map[string]bool{},
	}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = store.Append(ctx, value)
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %#v", err)
	}
}
