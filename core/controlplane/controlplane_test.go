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


func TestEventStoreReclaimsStaleAppendLock(t *testing.T) {
	root := t.TempDir()
	store, err := NewEventStore(root)
	if err != nil {
		t.Fatal(err)
	}
	runDir, err := store.ensureRunDir("run-stale")
	if err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(runDir, ".append.lock")
	if err := os.WriteFile(lockPath, []byte("stale\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-10 * time.Minute)
	if err := os.Chtimes(lockPath, old, old); err != nil {
		t.Fatal(err)
	}

	value, err := NewEvent(NewEventRequest{
		RunID: "run-stale",
		Type:  EventRunStarted,
		Data:  map[string]bool{"ok": true},
	}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Append(context.Background(), value); err != nil {
		t.Fatal(err)
	}
	events, err := store.List("run-stale")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %#v", events)
	}
}


func TestProjectorAllowsPromotionApprovalAfterRunCompleted(t *testing.T) {
	now := time.Now().UTC()
	types := []EventType{
		EventRunStarted,
		EventApprovalRequested,
		EventRunCompleted,
		EventApprovalGranted,
	}
	events := make([]Event, 0, len(types))
	for i, typ := range types {
		value, err := NewEvent(NewEventRequest{
			RunID: "run-post-approval",
			Type:  typ,
			Data:  map[string]bool{"ok": true},
		}, now.Add(time.Duration(i)*time.Second))
		if err != nil {
			t.Fatal(err)
		}
		value.Sequence = uint64(i + 1)
		events = append(events, value)
	}
	state, err := Project(events)
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != RunCompleted || state.PendingApprovals != 0 {
		t.Fatalf("state = %#v", state)
	}
}

func TestProjectorStillRejectsTaskMutationAfterRunCompleted(t *testing.T) {
	now := time.Now().UTC()
	started, _ := NewEvent(NewEventRequest{RunID: "run-terminal", Type: EventRunStarted, Data: map[string]bool{}}, now)
	completed, _ := NewEvent(NewEventRequest{RunID: "run-terminal", Type: EventRunCompleted, Data: map[string]bool{}}, now.Add(time.Second))
	task, _ := NewEvent(NewEventRequest{
		RunID: "run-terminal", Type: EventTaskStarted, TaskID: "late", AgentID: "backend-engineer",
		Data: map[string]bool{},
	}, now.Add(2*time.Second))
	started.Sequence = 1
	completed.Sequence = 2
	task.Sequence = 3
	if _, err := Project([]Event{started, completed, task}); err == nil {
		t.Fatal("expected post-terminal task rejection")
	}
}


func TestEventStoreRunIDsRejectsSymlinkRunEntry(t *testing.T) {
	root := t.TempDir()
	store, err := NewEventStore(root)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "run-link")
	if err := os.Symlink(target, link); err != nil {
		if os.IsPermission(err) {
			t.Skip("symlink creation not permitted")
		}
		t.Fatal(err)
	}
	if _, err := store.RunIDs(); err == nil {
		t.Fatal("expected symlink run entry rejection")
	}
}


func TestProjectorTracksCancelRequestUntilRunCancels(t *testing.T) {
	now := time.Now().UTC()
	types := []EventType{EventRunStarted, EventRunCancelRequested, EventRunCancelled}
	events := make([]Event, 0, len(types))
	for i, typ := range types {
		value, err := NewEvent(NewEventRequest{
			RunID: "run-cancel",
			Type:  typ,
			Data:  map[string]bool{"ok": true},
		}, now.Add(time.Duration(i)*time.Second))
		if err != nil {
			t.Fatal(err)
		}
		value.Sequence = uint64(i + 1)
		events = append(events, value)
	}
	pending, err := Project(events[:2])
	if err != nil {
		t.Fatal(err)
	}
	if pending.Status != RunRunning || !pending.CancelRequested {
		t.Fatalf("pending=%#v", pending)
	}
	final, err := Project(events)
	if err != nil {
		t.Fatal(err)
	}
	if final.Status != RunCancelled || final.CancelRequested {
		t.Fatalf("final=%#v", final)
	}
}


func TestProjectorTracksMutationPipelinePhases(t *testing.T) {
	now := time.Now().UTC()
	types := []EventType{
		EventRunStarted,
		EventProposalStarted,
		EventProposalCompleted,
		EventStageStarted,
		EventStageCompleted,
		EventIntegrationStarted,
		EventIntegrationCompleted,
	}
	events := make([]Event, 0, len(types))
	for i, typ := range types {
		value, err := NewEvent(NewEventRequest{
			RunID: "run-pipeline",
			Type:  typ,
			Data:  map[string]string{"phase": "source"},
		}, now.Add(time.Duration(i)*time.Second))
		if err != nil {
			t.Fatal(err)
		}
		value.Sequence = uint64(i + 1)
		events = append(events, value)
	}
	state, err := Project(events)
	if err != nil {
		t.Fatal(err)
	}
	if state.Pipeline.Proposal != "completed" ||
		state.Pipeline.Stage != "completed" ||
		state.Pipeline.Integration != "completed" {
		t.Fatalf("pipeline=%#v", state.Pipeline)
	}
}

func TestProjectorTracksFailedMutationPipelinePhase(t *testing.T) {
	now := time.Now().UTC()
	started, err := NewEvent(NewEventRequest{
		RunID: "run-pipeline-failed", Type: EventRunStarted, Data: map[string]bool{},
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	proposalStarted, err := NewEvent(NewEventRequest{
		RunID: "run-pipeline-failed", Type: EventProposalStarted, Data: map[string]bool{},
	}, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	proposalFailed, err := NewEvent(NewEventRequest{
		RunID: "run-pipeline-failed", Type: EventProposalFailed, Data: map[string]bool{},
	}, now.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	started.Sequence = 1
	proposalStarted.Sequence = 2
	proposalFailed.Sequence = 3
	state, err := Project([]Event{started, proposalStarted, proposalFailed})
	if err != nil {
		t.Fatal(err)
	}
	if state.Pipeline.Proposal != "failed" {
		t.Fatalf("pipeline=%#v", state.Pipeline)
	}
	if state.Pipeline.Stage != "" || state.Pipeline.Integration != "" {
		t.Fatalf("unexpected downstream phase state=%#v", state.Pipeline)
	}
}


func TestProjectorRepresentsFailedTerminalRun(t *testing.T) {
	now := time.Now().UTC()
	started, err := NewEvent(NewEventRequest{
		RunID: "run-failed", Type: EventRunStarted, Data: map[string]bool{"ok": true},
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	failed, err := NewEvent(NewEventRequest{
		RunID: "run-failed", Type: EventRunFailed, Data: map[string]bool{"ok": false},
	}, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	started.Sequence = 1
	failed.Sequence = 2
	state, err := Project([]Event{started, failed})
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != RunFailed {
		t.Fatalf("state=%#v", state)
	}
	late, err := NewEvent(NewEventRequest{
		RunID: "run-failed", Type: EventTaskStarted, TaskID: "late", AgentID: "backend-engineer",
		Data: map[string]bool{"ok": true},
	}, now.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	late.Sequence = 3
	if _, err := Project([]Event{started, failed, late}); err == nil {
		t.Fatal("expected post-failure task event rejection")
	}
}
