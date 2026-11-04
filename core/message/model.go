package message

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

type Kind string

const (
	KindRequest   Kind = "request"
	KindResponse  Kind = "response"
	KindQuestion  Kind = "question"
	KindAnswer    Kind = "answer"
	KindFinding   Kind = "finding"
	KindProposal  Kind = "proposal"
	KindChallenge Kind = "challenge"
	KindDecision  Kind = "decision"
	KindHandoff   Kind = "handoff"
	KindBlocker   Kind = "blocker"
	KindStatus    Kind = "status"
)

type Priority string

const (
	PriorityLow      Priority = "low"
	PriorityNormal   Priority = "normal"
	PriorityHigh     Priority = "high"
	PriorityCritical Priority = "critical"
)

type TargetKind string

const (
	TargetAgent        TargetKind = "agent"
	TargetRole         TargetKind = "role"
	TargetOrchestrator TargetKind = "orchestrator"
	TargetBroadcast    TargetKind = "broadcast"
)

type Target struct {
	Kind TargetKind `json:"kind"`
	ID   string     `json:"id,omitempty"`
}

type Message struct {
	SchemaVersion int             `json:"schema_version"`
	ID            string          `json:"id"`
	Sequence      uint64          `json:"sequence"`
	RunID         string          `json:"run_id"`
	TaskID        string          `json:"task_id"`
	FromAgent     string          `json:"from_agent"`
	To            Target          `json:"to"`
	Kind          Kind            `json:"kind"`
	Priority      Priority        `json:"priority"`
	ReplyTo       string          `json:"reply_to,omitempty"`
	ArtifactRefs  []string        `json:"artifact_refs,omitempty"`
	Payload       json.RawMessage `json:"payload"`
	Hop           int             `json:"hop"`
	CreatedAt     time.Time       `json:"created_at"`
	ExpiresAt     *time.Time      `json:"expires_at,omitempty"`
}

type NewRequest struct {
	RunID         string
	TaskID        string
	FromAgent     string
	To            Target
	Kind          Kind
	Priority      Priority
	ReplyTo       string
	ArtifactRefs  []string
	Payload       any
	Hop           int
	ExpiresAt     *time.Time
	AllowBroadcast bool
}

func New(request NewRequest, now time.Time) (Message, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if request.Priority == "" {
		request.Priority = PriorityNormal
	}
	if request.Hop < 0 || request.Hop > 8 {
		return Message{}, errors.New("message hop must be between 0 and 8")
	}
	if request.To.Kind == TargetBroadcast && !request.AllowBroadcast {
		return Message{}, errors.New("broadcast requires explicit authorization")
	}
	payload, err := json.Marshal(request.Payload)
	if err != nil {
		return Message{}, fmt.Errorf("encode message payload: %w", err)
	}
	id, err := newID()
	if err != nil {
		return Message{}, err
	}
	value := Message{
		SchemaVersion: SchemaVersion,
		ID:            id,
		RunID:         strings.TrimSpace(request.RunID),
		TaskID:        strings.TrimSpace(request.TaskID),
		FromAgent:     strings.TrimSpace(request.FromAgent),
		To: Target{
			Kind: request.To.Kind,
			ID:   strings.TrimSpace(request.To.ID),
		},
		Kind:         request.Kind,
		Priority:     request.Priority,
		ReplyTo:      strings.TrimSpace(request.ReplyTo),
		ArtifactRefs: sortedUnique(request.ArtifactRefs),
		Payload:      payload,
		Hop:          request.Hop,
		CreatedAt:    now.UTC(),
	}
	if request.ExpiresAt != nil {
		expiry := request.ExpiresAt.UTC()
		value.ExpiresAt = &expiry
	}
	if err := Validate(value); err != nil {
		return Message{}, err
	}
	return value, nil
}

func Validate(value Message) error {
	if value.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported message schema version: %d", value.SchemaVersion)
	}
	if !validMessageID(value.ID) {
		return errors.New("message id is invalid")
	}
	if strings.TrimSpace(value.RunID) == "" || strings.TrimSpace(value.TaskID) == "" {
		return errors.New("message run_id and task_id are required")
	}
	if strings.TrimSpace(value.FromAgent) == "" {
		return errors.New("message from_agent is required")
	}
	if !validKind(value.Kind) {
		return fmt.Errorf("unsupported message kind: %q", value.Kind)
	}
	if !validPriority(value.Priority) {
		return fmt.Errorf("unsupported message priority: %q", value.Priority)
	}
	if err := validateTarget(value.To); err != nil {
		return err
	}
	if value.ReplyTo != "" && !validMessageID(value.ReplyTo) {
		return errors.New("reply_to is not a valid message id")
	}
	for _, ref := range value.ArtifactRefs {
		if !validArtifactID(ref) {
			return fmt.Errorf("invalid artifact reference: %q", ref)
		}
	}
	if len(value.Payload) == 0 || !json.Valid(value.Payload) {
		return errors.New("message payload must be valid JSON")
	}
	if len(value.Payload) > 64*1024 {
		return errors.New("message payload exceeds 64 KiB")
	}
	if value.Hop < 0 || value.Hop > 8 {
		return errors.New("message hop must be between 0 and 8")
	}
	if value.CreatedAt.IsZero() {
		return errors.New("message created_at is required")
	}
	if value.ExpiresAt != nil && !value.ExpiresAt.After(value.CreatedAt) {
		return errors.New("message expires_at must be after created_at")
	}
	return nil
}

func (m Message) Expired(now time.Time) bool {
	return m.ExpiresAt != nil && !now.Before(*m.ExpiresAt)
}

func validateTarget(target Target) error {
	switch target.Kind {
	case TargetAgent, TargetRole:
		if strings.TrimSpace(target.ID) == "" {
			return fmt.Errorf("target %s requires an id", target.Kind)
		}
	case TargetOrchestrator, TargetBroadcast:
		if strings.TrimSpace(target.ID) != "" {
			return fmt.Errorf("target %s must not include an id", target.Kind)
		}
	default:
		return fmt.Errorf("unsupported message target: %q", target.Kind)
	}
	return nil
}

func validKind(value Kind) bool {
	switch value {
	case KindRequest, KindResponse, KindQuestion, KindAnswer, KindFinding, KindProposal,
		KindChallenge, KindDecision, KindHandoff, KindBlocker, KindStatus:
		return true
	default:
		return false
	}
}

func validPriority(value Priority) bool {
	switch value {
	case PriorityLow, PriorityNormal, PriorityHigh, PriorityCritical:
		return true
	default:
		return false
	}
}

func newID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate message id: %w", err)
	}
	return "message-" + hex.EncodeToString(value[:]), nil
}

func validMessageID(value string) bool {
	return validHexID(value, "message-")
}

func validArtifactID(value string) bool {
	return validHexID(value, "artifact-")
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
