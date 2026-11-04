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
