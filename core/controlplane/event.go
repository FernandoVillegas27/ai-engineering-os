package controlplane

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

const SchemaVersion = 1

type EventType string

const (
	EventRunStarted            EventType = "run.started"
	EventRunPauseRequested     EventType = "run.pause-requested"
	EventRunPaused             EventType = "run.paused"
	EventRunResumed            EventType = "run.resumed"
	EventRunRecovered          EventType = "run.recovered"
	EventRunCompleted          EventType = "run.completed"
	EventRunFailed             EventType = "run.failed"
	EventRunCancelled          EventType = "run.cancelled"
	EventRunCancelRequested     EventType = "run.cancel-requested"
	EventTaskCreated           EventType = "task.created"
	EventTaskAssigned          EventType = "task.assigned"
	EventTaskStarted           EventType = "task.started"
	EventTaskWaiting           EventType = "task.waiting"
	EventTaskBlocked           EventType = "task.blocked"
	EventTaskCompleted         EventType = "task.completed"
	EventTaskFailed            EventType = "task.failed"
	EventTaskCancelled         EventType = "task.cancelled"
	EventAgentMessageSent      EventType = "agent.message.sent"
	EventArtifactCreated       EventType = "artifact.created"
	EventCheckpointCreated     EventType = "checkpoint.created"
	EventProposalStarted       EventType = "proposal.started"
	EventProposalCompleted     EventType = "proposal.completed"
	EventProposalFailed        EventType = "proposal.failed"
	EventStageStarted          EventType = "stage.started"
	EventStageCompleted        EventType = "stage.completed"
	EventStageFailed           EventType = "stage.failed"
	EventIntegrationStarted    EventType = "integration.started"
	EventIntegrationCompleted  EventType = "integration.completed"
	EventIntegrationFailed     EventType = "integration.failed"
	EventVerificationStarted   EventType = "verification.started"
	EventVerificationPassed    EventType = "verification.passed"
	EventVerificationFailed    EventType = "verification.failed"
	EventBaselineCompared      EventType = "baseline.compared"
	EventCorrectionStarted     EventType = "correction.started"
	EventCorrectionCompleted   EventType = "correction.completed"
	EventCorrectionFailed      EventType = "correction.failed"
	EventReviewStarted         EventType = "review.started"
	EventReviewApproved        EventType = "review.approved"
	EventReviewRejected        EventType = "review.rejected"
	EventApprovalRequested     EventType = "approval.requested"
	EventApprovalGranted       EventType = "approval.granted"
	EventApprovalRejected      EventType = "approval.rejected"
	EventRuntimeSelected       EventType = "runtime.selected"
	EventModelEscalated        EventType = "model.escalated"
	EventToolStarted           EventType = "tool.started"
	EventToolCompleted         EventType = "tool.completed"
	EventToolFailed            EventType = "tool.failed"
)

type Event struct {
	SchemaVersion int             `json:"schema_version"`
	ID            string          `json:"id"`
	Sequence      uint64          `json:"sequence"`
	RunID         string          `json:"run_id"`
	Type          EventType       `json:"type"`
	TaskID        string          `json:"task_id,omitempty"`
	AgentID       string          `json:"agent_id,omitempty"`
	Summary       string          `json:"summary,omitempty"`
	ArtifactRefs  []string        `json:"artifact_refs,omitempty"`
	MessageRefs   []string        `json:"message_refs,omitempty"`
	Data          json.RawMessage `json:"data"`
	CreatedAt     time.Time       `json:"created_at"`
}

type NewEventRequest struct {
	RunID        string
	Type         EventType
	TaskID       string
	AgentID      string
	Summary      string
	ArtifactRefs []string
	MessageRefs  []string
	Data         any
}

func NewEvent(request NewEventRequest, now time.Time) (Event, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	data, err := json.Marshal(request.Data)
	if err != nil {
		return Event{}, fmt.Errorf("encode control-plane event data: %w", err)
	}
	id, err := newEventID()
	if err != nil {
		return Event{}, err
	}
	value := Event{
		SchemaVersion: SchemaVersion,
		ID:            id,
		RunID:         strings.TrimSpace(request.RunID),
		Type:          request.Type,
		TaskID:        strings.TrimSpace(request.TaskID),
		AgentID:       strings.TrimSpace(request.AgentID),
		Summary:       strings.TrimSpace(request.Summary),
		ArtifactRefs:  sortedUnique(request.ArtifactRefs),
		MessageRefs:   sortedUnique(request.MessageRefs),
		Data:          data,
		CreatedAt:     now.UTC(),
	}
	if err := ValidateEvent(value); err != nil {
		return Event{}, err
	}
	return value, nil
}

func ValidateEvent(value Event) error {
	if value.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported control-plane event schema version: %d", value.SchemaVersion)
	}
	if !validHexID(value.ID, "event-") {
		return errors.New("control-plane event id is invalid")
	}
	if strings.TrimSpace(value.RunID) == "" {
		return errors.New("control-plane event run_id is required")
	}
	if !validEventType(value.Type) {
		return fmt.Errorf("unsupported control-plane event type: %q", value.Type)
	}
	if taskEvent(value.Type) && strings.TrimSpace(value.TaskID) == "" {
		return fmt.Errorf("control-plane event %s requires task_id", value.Type)
	}
	if agentEvent(value.Type) && strings.TrimSpace(value.AgentID) == "" {
		return fmt.Errorf("control-plane event %s requires agent_id", value.Type)
	}
	if len(value.Summary) > 2000 {
		return errors.New("control-plane event summary exceeds 2000 characters")
	}
	if len(value.Data) == 0 || !json.Valid(value.Data) {
		return errors.New("control-plane event data must be valid JSON")
	}
	if len(value.Data) > 64*1024 {
		return errors.New("control-plane event data exceeds 64 KiB")
	}
	for _, ref := range value.ArtifactRefs {
		if !validHexID(ref, "artifact-") {
			return fmt.Errorf("invalid artifact reference: %q", ref)
		}
	}
	for _, ref := range value.MessageRefs {
		if !validHexID(ref, "message-") {
			return fmt.Errorf("invalid message reference: %q", ref)
		}
	}
	if value.CreatedAt.IsZero() {
		return errors.New("control-plane event created_at is required")
	}
	return nil
}

func taskEvent(value EventType) bool {
	switch value {
	case EventTaskCreated, EventTaskAssigned, EventTaskStarted, EventTaskWaiting,
		EventTaskBlocked, EventTaskCompleted, EventTaskFailed, EventTaskCancelled:
		return true
	default:
		return false
	}
}

func agentEvent(value EventType) bool {
	switch value {
	case EventTaskAssigned, EventTaskStarted, EventTaskWaiting, EventTaskBlocked,
		EventTaskCompleted, EventTaskFailed:
		return true
	default:
		return false
	}
}

func validEventType(value EventType) bool {
	switch value {
	case EventRunStarted, EventRunPauseRequested, EventRunPaused, EventRunResumed, EventRunRecovered, EventRunCompleted, EventRunFailed, EventRunCancelled, EventRunCancelRequested,
		EventTaskCreated, EventTaskAssigned, EventTaskStarted, EventTaskWaiting, EventTaskBlocked,
		EventTaskCompleted, EventTaskFailed, EventTaskCancelled, EventAgentMessageSent,
		EventArtifactCreated, EventCheckpointCreated, EventProposalStarted, EventProposalCompleted, EventProposalFailed,
		EventStageStarted, EventStageCompleted, EventStageFailed,
		EventIntegrationStarted, EventIntegrationCompleted, EventIntegrationFailed,
		EventVerificationStarted, EventVerificationPassed, EventVerificationFailed,
		EventBaselineCompared, EventCorrectionStarted, EventCorrectionCompleted, EventCorrectionFailed,
		EventReviewStarted, EventReviewApproved, EventReviewRejected, EventApprovalRequested,
		EventApprovalGranted, EventApprovalRejected, EventRuntimeSelected, EventModelEscalated,
		EventToolStarted, EventToolCompleted, EventToolFailed:
		return true
	default:
		return false
	}
}

func newEventID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate control-plane event id: %w", err)
	}
	return "event-" + hex.EncodeToString(value[:]), nil
}

func validHexID(value, prefix string) bool {
	if !strings.HasPrefix(value, prefix) {
		return false
	}
	suffix := strings.TrimPrefix(value, prefix)
	if len(suffix) != 32 {
		return false
	}
	for _, r := range suffix {
		if !(r >= 'a' && r <= 'f' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

func sortedUnique(values []string) []string {
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			seen[value] = true
		}
	}
	out := make([]string, 0, len(seen))
	for value := range seen {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
