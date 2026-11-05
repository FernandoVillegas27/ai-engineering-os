package collaboration

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/artifact"
	"github.com/FernandoVillegas27/ai-engineering-os/core/coordinator"
	"github.com/FernandoVillegas27/ai-engineering-os/core/message"
	"github.com/FernandoVillegas27/ai-engineering-os/core/scheduler"
)

type workerFunc func(context.Context, scheduler.Task) (coordinator.WorkResult, error)

func (f workerFunc) Execute(ctx context.Context, task scheduler.Task) (coordinator.WorkResult, error) {
	return f(ctx, task)
}

func TestHandoffWorkerPersistsArtifactAndMessagesToDependents(t *testing.T) {
	artifactStore, err := artifact.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	messageBus, err := message.NewBus(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	tasks := []scheduler.Task{
		{ID: "architect", AgentID: "architect"},
		{ID: "backend", AgentID: "backend-engineer", DependsOn: []string{"architect"}},
		{ID: "frontend", AgentID: "frontend-engineer", DependsOn: []string{"architect"}},
	}
	now := time.Date(2026, 11, 4, 20, 0, 0, 0, time.UTC)
	worker := HandoffWorker{
		Inner: workerFunc(func(context.Context, scheduler.Task) (coordinator.WorkResult, error) {
			return coordinator.WorkResult{Summary: "API boundaries defined"}, nil
		}),
		RunID:         "run-1",
		ArtifactStore: artifactStore,
		MessageBus:    messageBus,
		Dependents:    BuildDependents(tasks),
		Now:           func() time.Time { return now },
	}

	result, err := worker.Execute(context.Background(), tasks[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(result.ArtifactRefs) != 1 {
		t.Fatalf("artifact refs = %#v", result.ArtifactRefs)
	}
	stored, err := artifactStore.Load(result.ArtifactRefs[0])
	if err != nil {
		t.Fatal(err)
	}
	if stored.ProducerAgent != "architect" || stored.Type != artifact.TypeHandoff {
		t.Fatalf("artifact = %#v", stored)
	}
	var handoff artifact.HandoffPayload
	if err := json.Unmarshal(stored.Payload, &handoff); err != nil {
		t.Fatal(err)
	}
	if handoff.SchemaVersion != artifact.LifecyclePayloadSchemaVersion ||
		handoff.TaskID != "architect" ||
		handoff.AgentID != "architect" ||
		handoff.Summary != "API boundaries defined" {
		t.Fatalf("handoff payload = %#v", handoff)
	}
	if artifact.PayloadSchemaID(stored.Type, stored.Payload) != "artifact/handoff/v1" {
		t.Fatalf("artifact schema = %s", artifact.PayloadSchemaID(stored.Type, stored.Payload))
	}

	messages, err := messageBus.List("run-1", true, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || len(result.MessageRefs) != 2 {
		t.Fatalf("messages=%#v result=%#v", messages, result)
	}
	if messages[0].FromAgent != "architect" || messages[1].FromAgent != "architect" {
		t.Fatalf("messages = %#v", messages)
	}
	targets := map[string]bool{}
	for _, msg := range messages {
		targets[msg.To.ID] = true
		if len(msg.ArtifactRefs) != 1 || msg.ArtifactRefs[0] != stored.ID {
			t.Fatalf("message refs = %#v", msg.ArtifactRefs)
		}
		var payload message.HandoffPayload
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		if payload.SchemaVersion != message.PayloadSchemaVersion ||
			payload.FromTask != "architect" ||
			payload.Summary != stored.Summary {
			t.Fatalf("message handoff payload = %#v", payload)
		}
		if message.PayloadSchemaID(msg.Kind, msg.Payload) != "message/handoff/v1" {
			t.Fatalf("message schema = %s", message.PayloadSchemaID(msg.Kind, msg.Payload))
		}
	}
	if !targets["backend-engineer"] || !targets["frontend-engineer"] {
		t.Fatalf("targets = %#v", targets)
	}
}

func TestHandoffWorkerWithoutDependentsStillPersistsArtifact(t *testing.T) {
	artifactStore, _ := artifact.NewStore(t.TempDir())
	messageBus, _ := message.NewBus(t.TempDir())
	worker := HandoffWorker{
		Inner: workerFunc(func(context.Context, scheduler.Task) (coordinator.WorkResult, error) {
			return coordinator.WorkResult{Summary: "review complete"}, nil
		}),
		RunID:         "run-2",
		ArtifactStore: artifactStore,
		MessageBus:    messageBus,
		Dependents:    map[string][]scheduler.Task{},
	}
	result, err := worker.Execute(context.Background(), scheduler.Task{ID: "review", AgentID: "reviewer"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.ArtifactRefs) != 1 || len(result.MessageRefs) != 0 {
		t.Fatalf("result = %#v", result)
	}
}

func TestHandoffWorkerDoesNotPublishOnInnerFailure(t *testing.T) {
	artifactStore, _ := artifact.NewStore(t.TempDir())
	messageBus, _ := message.NewBus(t.TempDir())
	worker := HandoffWorker{
		Inner: workerFunc(func(context.Context, scheduler.Task) (coordinator.WorkResult, error) {
			return coordinator.WorkResult{}, errors.New("worker failed")
		}),
		RunID:         "run-3",
		ArtifactStore: artifactStore,
		MessageBus:    messageBus,
	}
	if _, err := worker.Execute(context.Background(), scheduler.Task{ID: "backend", AgentID: "backend-engineer"}); err == nil {
		t.Fatal("expected worker error")
	}
	messages, err := messageBus.List("run-3", true, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 0 {
		t.Fatalf("messages = %#v", messages)
	}
}

func TestBuildDependentsIsDeterministic(t *testing.T) {
	dependents := BuildDependents([]scheduler.Task{
		{ID: "qa", AgentID: "qa-engineer", DependsOn: []string{"backend", "frontend"}},
		{ID: "backend", AgentID: "backend-engineer"},
		{ID: "frontend", AgentID: "frontend-engineer"},
	})
	if len(dependents["backend"]) != 1 || dependents["backend"][0].ID != "qa" {
		t.Fatalf("backend dependents = %#v", dependents["backend"])
	}
	if len(dependents["frontend"]) != 1 || dependents["frontend"][0].ID != "qa" {
		t.Fatalf("frontend dependents = %#v", dependents["frontend"])
	}
}
