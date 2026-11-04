package main

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/artifact"
	"github.com/FernandoVillegas27/ai-engineering-os/core/collaboration"
	"github.com/FernandoVillegas27/ai-engineering-os/core/controlplane"
	"github.com/FernandoVillegas27/ai-engineering-os/core/coordinator"
	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/message"
	"github.com/FernandoVillegas27/ai-engineering-os/core/mutation"
	"github.com/FernandoVillegas27/ai-engineering-os/core/scheduler"
)

type mutationTaskEventSink struct {
	runID string
	store *controlplane.EventStore
}

func (s mutationTaskEventSink) Record(ctx context.Context, input coordinator.Event) error {
	if s.store == nil {
		return fmt.Errorf("mutation task event sink requires store")
	}
	var eventType controlplane.EventType
	switch input.Type {
	case coordinator.EventTaskStarted:
		eventType = controlplane.EventTaskStarted
	case coordinator.EventTaskCompleted:
		eventType = controlplane.EventTaskCompleted
	case coordinator.EventTaskFailed:
		eventType = controlplane.EventTaskFailed
	case coordinator.EventRunStarted, coordinator.EventRunCompleted, coordinator.EventRunCancelled:
		return nil
	default:
		return nil
	}
	event, err := controlplane.NewEvent(controlplane.NewEventRequest{
		RunID:        s.runID,
		Type:         eventType,
		TaskID:       input.TaskID,
		AgentID:      input.AgentID,
		Summary:      input.Summary,
		ArtifactRefs: input.ArtifactRefs,
		MessageRefs:  input.MessageRefs,
		Data: map[string]any{
			"phase": "pre-stage-collaboration",
		},
	}, time.Now().UTC())
	if err != nil {
		return err
	}
	if _, err := s.store.Append(ctx, event); err != nil {
		return err
	}
	if input.Type == coordinator.EventTaskCompleted {
		for _, ref := range input.ArtifactRefs {
			value, err := controlplane.NewEvent(controlplane.NewEventRequest{
				RunID:        s.runID,
				Type:         controlplane.EventArtifactCreated,
				TaskID:       input.TaskID,
				AgentID:      input.AgentID,
				Summary:      ref,
				ArtifactRefs: []string{ref},
				Data: map[string]any{"phase": "pre-stage-collaboration"},
			}, time.Now().UTC())
			if err != nil {
				return err
			}
			if _, err := s.store.Append(ctx, value); err != nil {
				return err
			}
		}
		for _, ref := range input.MessageRefs {
			value, err := controlplane.NewEvent(controlplane.NewEventRequest{
				RunID:       s.runID,
				Type:        controlplane.EventAgentMessageSent,
				TaskID:      input.TaskID,
				AgentID:     input.AgentID,
				Summary:     ref,
				MessageRefs: []string{ref},
				Data: map[string]any{"phase": "pre-stage-collaboration"},
			}, time.Now().UTC())
			if err != nil {
				return err
			}
			if _, err := s.store.Append(ctx, value); err != nil {
				return err
			}
		}
	}
	return nil
}

func executeMutationPreStage(
	ctx context.Context,
	options runExecuteOptions,
	runID string,
	plan mutation.PreStagePlan,
	mutating []execution.WorkUnit,
	artifactStore *artifact.Store,
	messageBus *message.Bus,
	eventStore *controlplane.EventStore,
	routing modelRoutingOptions,
) ([]execution.WorkUnit, error) {
	if len(plan.Tasks) > 0 {
		graph, err := scheduler.New(plan.Tasks)
		if err != nil {
			return nil, err
		}
		units := map[string]execution.WorkUnit{}
		workers := map[string]coordinator.Worker{}
		for _, unit := range plan.Units {
			units[unit.ID] = unit
		}
		for _, unit := range plan.Units {
			runtime, _, err := executionRuntimeFor(options.runtime, routing)
			if err != nil {
				return nil, err
			}
			inner := coordinator.ExecutionWorker{
				Executor: execution.RuntimeExecutor{Runtime: runtime},
				Units:    units,
				ContextProvider: collaboration.InboxContextProvider{
					RunID:         runID,
					MessageBus:    messageBus,
					ArtifactStore: artifactStore,
				},
			}
			workers[unit.AgentID] = collaboration.HandoffWorker{
				Inner:         inner,
				RunID:         runID,
				ArtifactStore: artifactStore,
				MessageBus:    messageBus,
				Dependents:    plan.Dependents,
			}
		}
		result, err := (coordinator.Coordinator{
			Config:  coordinator.Config{MaxConcurrency: options.maxConcurrency},
			Workers: workers,
			Events:  mutationTaskEventSink{runID: runID, store: eventStore},
		}).Run(ctx, graph)
		if err != nil {
			return nil, fmt.Errorf("pre-stage collaboration: %w", err)
		}
		if result.Progress.Failed > 0 || result.Progress.Blocked > 0 || result.Progress.Cancelled > 0 {
			return nil, fmt.Errorf("pre-stage collaboration did not complete cleanly")
		}
	}

	provider := collaboration.InboxContextProvider{
		RunID:         runID,
		MessageBus:    messageBus,
		ArtifactStore: artifactStore,
	}
	out := make([]execution.WorkUnit, len(mutating))
	for i, unit := range mutating {
		contextValue, err := provider.Context(ctx, scheduler.Task{ID: unit.ID, AgentID: unit.AgentID})
		if err != nil {
			return nil, fmt.Errorf("load pre-stage inbox for %s: %w", unit.ID, err)
		}
		unit.CollaborationContext = contextValue
		out[i] = unit
	}
	return out, nil
}


func mutationParticipants(groups ...[]execution.WorkUnit) []execution.WorkUnit {
	byID := map[string]execution.WorkUnit{}
	for _, group := range groups {
		for _, unit := range group {
			byID[unit.ID] = unit
		}
	}
	out := make([]execution.WorkUnit, 0, len(byID))
	for _, unit := range byID {
		out = append(out, unit)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
