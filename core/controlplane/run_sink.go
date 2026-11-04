package controlplane

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/coordinator"
	"github.com/FernandoVillegas27/ai-engineering-os/core/scheduler"
)

type RunCoordinatorSink struct {
	RunID string
	Store *EventStore
	Tasks []scheduler.Task
	Now   func() time.Time

	mu          sync.Mutex
	initialized bool
}

func (s *RunCoordinatorSink) Record(ctx context.Context, input coordinator.Event) error {
	if s == nil || s.Store == nil {
		return fmt.Errorf("control-plane run sink requires event store")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	if s.Now != nil {
		now = s.Now().UTC()
	}

	if !s.initialized {
		if input.Type != coordinator.EventRunStarted {
			return fmt.Errorf("control-plane run sink must initialize from run.started")
		}
		if err := s.append(ctx, EventRunStarted, "", "", input.Summary, input.ArtifactRefs, input.MessageRefs, now); err != nil {
			return err
		}
		for _, task := range s.Tasks {
			if err := s.append(ctx, EventTaskCreated, task.ID, "", "", nil, nil, now); err != nil {
				return err
			}
			if err := s.append(ctx, EventTaskAssigned, task.ID, task.AgentID, "", nil, nil, now); err != nil {
				return err
			}
		}
		s.initialized = true
		return nil
	}

	eventType, err := coordinatorEventType(input.Type)
	if err != nil {
		return err
	}
	return s.append(ctx, eventType, input.TaskID, input.AgentID, input.Summary, input.ArtifactRefs, input.MessageRefs, now)
}

func (s *RunCoordinatorSink) append(
	ctx context.Context,
	eventType EventType,
	taskID string,
	agentID string,
	summary string,
	artifactRefs []string,
	messageRefs []string,
	now time.Time,
) error {
	event, err := NewEvent(NewEventRequest{
		RunID:        s.RunID,
		Type:         eventType,
		TaskID:       taskID,
		AgentID:      agentID,
		Summary:      summary,
		ArtifactRefs: artifactRefs,
		MessageRefs:  messageRefs,
		Data: map[string]any{
			"source": "coordinator",
		},
	}, now)
	if err != nil {
		return err
	}
	_, err = s.Store.Append(ctx, event)
	return err
}
