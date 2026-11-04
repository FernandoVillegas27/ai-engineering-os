package controlplane

import (
	"context"
	"fmt"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/coordinator"
)

type CoordinatorSink struct {
	RunID string
	Store *EventStore
	Now   func() time.Time
}

func (s CoordinatorSink) Record(ctx context.Context, input coordinator.Event) error {
	if s.Store == nil {
		return fmt.Errorf("control-plane event store is required")
	}
	eventType, err := coordinatorEventType(input.Type)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if s.Now != nil {
		now = s.Now().UTC()
	}
	event, err := NewEvent(NewEventRequest{
		RunID:        s.RunID,
		Type:         eventType,
		TaskID:       input.TaskID,
		AgentID:      input.AgentID,
		Summary:      input.Summary,
		ArtifactRefs: input.ArtifactRefs,
		MessageRefs:  input.MessageRefs,
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

func coordinatorEventType(value coordinator.EventType) (EventType, error) {
	switch value {
	case coordinator.EventRunStarted:
		return EventRunStarted, nil
	case coordinator.EventTaskStarted:
		return EventTaskStarted, nil
	case coordinator.EventTaskCompleted:
		return EventTaskCompleted, nil
	case coordinator.EventTaskFailed:
		return EventTaskFailed, nil
	case coordinator.EventRunCompleted:
		return EventRunCompleted, nil
	case coordinator.EventRunCancelled:
		return EventRunCancelled, nil
	default:
		return "", fmt.Errorf("unsupported coordinator event type: %s", value)
	}
}
