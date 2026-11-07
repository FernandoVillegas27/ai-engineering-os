package controlplane

import (
	"context"
	"testing"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/coordinator"
	"github.com/FernandoVillegas27/ai-engineering-os/core/scheduler"
)

func TestRunCoordinatorSinkSeedsTaskGraphBeforeLifecycleEvents(t *testing.T) {
	store, err := NewEventStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)
	sink := &RunCoordinatorSink{
		RunID: "run-seeded",
		Store: store,
		Tasks: []scheduler.Task{
			{ID: "backend", AgentID: "backend-engineer"},
			{ID: "frontend", AgentID: "frontend-engineer"},
		},
		Now: func() time.Time { return now },
	}

	if err := sink.Record(context.Background(), coordinator.Event{Type: coordinator.EventRunStarted}); err != nil {
		t.Fatal(err)
	}
	if err := sink.Record(context.Background(), coordinator.Event{
		Type:    coordinator.EventTaskStarted,
		TaskID:  "backend",
		AgentID: "backend-engineer",
	}); err != nil {
		t.Fatal(err)
	}

	events, err := store.List("run-seeded")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 6 {
		t.Fatalf("events = %#v", events)
	}
	if events[0].Type != EventRunStarted ||
		events[1].Type != EventTaskCreated ||
		events[2].Type != EventTaskAssigned ||
		events[3].Type != EventTaskCreated ||
		events[4].Type != EventTaskAssigned ||
		events[5].Type != EventTaskStarted {
		t.Fatalf("events = %#v", events)
	}

	state, err := Project(events)
	if err != nil {
		t.Fatal(err)
	}
	if state.Progress.Total != 2 || state.Progress.Running != 1 || state.Progress.Assigned != 1 {
		t.Fatalf("progress = %#v", state.Progress)
	}
	if state.Agents["backend-engineer"].Status != AgentRunning {
		t.Fatalf("backend = %#v", state.Agents["backend-engineer"])
	}
	if state.Agents["frontend-engineer"].Status != AgentWaiting {
		t.Fatalf("frontend = %#v", state.Agents["frontend-engineer"])
	}
}


func TestRunCoordinatorSinkProjectsArtifactAndMessageReferences(t *testing.T) {
	store, err := NewEventStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sink := &RunCoordinatorSink{
		RunID: "run-refs",
		Store: store,
		Tasks: []scheduler.Task{
			{ID: "architect", AgentID: "architect"},
		},
	}
	if err := sink.Record(context.Background(), coordinator.Event{Type: coordinator.EventRunStarted}); err != nil {
		t.Fatal(err)
	}
	if err := sink.Record(context.Background(), coordinator.Event{
		Type:         coordinator.EventTaskCompleted,
		TaskID:       "architect",
		AgentID:      "architect",
		Summary:      "done",
		ArtifactRefs: []string{"artifact-11111111111111111111111111111111"},
		MessageRefs:  []string{"message-22222222222222222222222222222222"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := sink.Record(context.Background(), coordinator.Event{Type: coordinator.EventRunCompleted}); err != nil {
		t.Fatal(err)
	}

	events, err := store.List("run-refs")
	if err != nil {
		t.Fatal(err)
	}
	state, err := Project(events)
	if err != nil {
		t.Fatal(err)
	}
	if state.ArtifactCount != 1 || state.MessageCount != 1 {
		t.Fatalf("state = %#v", state)
	}
}


func TestRunCoordinatorSinkProjectsTaskDependenciesAndScopes(t *testing.T) {
	store, err := NewEventStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sink := &RunCoordinatorSink{
		RunID: "run-graph",
		Store: store,
		Tasks: []scheduler.Task{
			{ID: "architect", AgentID: "architect"},
			{
				ID: "backend", AgentID: "backend-engineer",
				DependsOn: []string{"architect"},
				Mutating: true,
				Scopes: []string{"backend/api", "backend/service"},
				ToolOperations: map[string][]string{
					"filesystem": {"read-files", "write-files"},
					"github": {"read-repository"},
				},
			},
		},
	}
	if err := sink.Record(context.Background(), coordinator.Event{Type: coordinator.EventRunStarted}); err != nil {
		t.Fatal(err)
	}
	events, err := store.List("run-graph")
	if err != nil {
		t.Fatal(err)
	}
	state, err := Project(events)
	if err != nil {
		t.Fatal(err)
	}
	backend := state.Tasks["backend"]
	if len(backend.DependsOn) != 1 || backend.DependsOn[0] != "architect" {
		t.Fatalf("backend deps = %#v", backend.DependsOn)
	}
	if !backend.Mutating {
		t.Fatal("backend mutating flag was lost")
	}
	if len(backend.Scopes) != 2 || backend.Scopes[0] != "backend/api" || backend.Scopes[1] != "backend/service" {
		t.Fatalf("backend scopes = %#v", backend.Scopes)
	}
	if len(state.Edges) != 1 || state.Edges[0].From != "architect" || state.Edges[0].To != "backend" {
		t.Fatalf("edges = %#v", state.Edges)
	}
	if got := backend.ToolOperations["github"]; len(got) != 1 || got[0] != "read-repository" {
		t.Fatalf("backend GitHub authority = %#v", backend.ToolOperations)
	}
	if got := backend.ToolOperations["filesystem"]; len(got) != 2 ||
		got[0] != "read-files" || got[1] != "write-files" {
		t.Fatalf("backend filesystem authority = %#v", backend.ToolOperations)
	}
}


func TestRunCoordinatorSinkResumeDoesNotRecreateTasks(t *testing.T) {
	store, err := NewEventStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	started, err := NewEvent(NewEventRequest{
		RunID: "run-resume", Type: EventRunStarted, Data: map[string]bool{"ok": true},
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Append(context.Background(), started); err != nil {
		t.Fatal(err)
	}
	created, err := NewEvent(NewEventRequest{
		RunID: "run-resume", Type: EventTaskCreated, TaskID: "task-1",
		Data: map[string]any{"depends_on": []string{}, "mutating": false, "scopes": []string{}},
	}, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Append(context.Background(), created); err != nil {
		t.Fatal(err)
	}
	paused, err := NewEvent(NewEventRequest{
		RunID: "run-resume", Type: EventRunPaused, Data: map[string]bool{"ok": true},
	}, now.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Append(context.Background(), paused); err != nil {
		t.Fatal(err)
	}

	sink := &RunCoordinatorSink{
		RunID: "run-resume",
		Store:  store,
		Tasks:  []scheduler.Task{{ID: "task-1", AgentID: "researcher"}},
		Resume: true,
	}
	if err := sink.Record(context.Background(), coordinator.Event{Type: coordinator.EventRunResumed}); err != nil {
		t.Fatal(err)
	}
	if err := sink.Record(context.Background(), coordinator.Event{
		Type: coordinator.EventTaskCompleted, TaskID: "task-1", AgentID: "researcher",
	}); err != nil {
		t.Fatal(err)
	}
	if err := sink.Record(context.Background(), coordinator.Event{Type: coordinator.EventRunCompleted}); err != nil {
		t.Fatal(err)
	}
	events, err := store.List("run-resume")
	if err != nil {
		t.Fatal(err)
	}
	createdCount := 0
	for _, event := range events {
		if event.Type == EventTaskCreated {
			createdCount++
		}
	}
	if createdCount != 1 {
		t.Fatalf("task.created count=%d events=%#v", createdCount, events)
	}
	if events[3].Type != EventRunResumed {
		t.Fatalf("events=%#v", events)
	}
}


func TestProjectorAcceptsLegacyTaskCreatedWithoutToolOperations(t *testing.T) {
	now := time.Now().UTC()
	started, err := NewEvent(NewEventRequest{
		RunID: "legacy-tools", Type: EventRunStarted, Data: map[string]bool{"ok": true},
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	created, err := NewEvent(NewEventRequest{
		RunID: "legacy-tools", Type: EventTaskCreated, TaskID: "task-1",
		Data: map[string]any{
			"depends_on": []string{},
			"mutating": false,
			"scopes": []string{},
		},
	}, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	started.Sequence = 1
	created.Sequence = 2
	state, err := Project([]Event{started, created})
	if err != nil {
		t.Fatal(err)
	}
	if state.Tasks["task-1"].ToolOperations != nil {
		t.Fatalf("legacy task unexpectedly gained tool authority: %#v", state.Tasks["task-1"])
	}
}
