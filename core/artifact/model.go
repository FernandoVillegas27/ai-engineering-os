package artifact

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

type Type string

const (
	TypeTaskBrief            Type = "task-brief"
	TypeArchitectureDecision Type = "architecture-decision"
	TypeResearchNote         Type = "research-note"
	TypeImplementationPlan   Type = "implementation-plan"
	TypeChangeProposal       Type = "change-proposal"
	TypeCodePatchReference   Type = "code-patch-reference"
	TypeTestPlan             Type = "test-plan"
	TypeVerificationReport   Type = "verification-report"
	TypeReviewReport         Type = "review-report"
	TypeRiskAssessment       Type = "risk-assessment"
	TypeQuestion             Type = "question"
	TypeDecision             Type = "decision"
	TypeHandoff              Type = "handoff"
	TypeAPIContract          Type = "api-contract"
)

type Status string

const (
	StatusDraft      Status = "draft"
	StatusActive     Status = "active"
	StatusSuperseded Status = "superseded"
	StatusAccepted   Status = "accepted"
	StatusRejected   Status = "rejected"
)

type EvidenceRef struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

type Artifact struct {
	SchemaVersion int             `json:"schema_version"`
	ID            string          `json:"id"`
	RunID         string          `json:"run_id"`
	TaskID        string          `json:"task_id"`
	ProducerAgent string          `json:"producer_agent"`
	Type          Type            `json:"type"`
	Status        Status          `json:"status"`
	Summary       string          `json:"summary"`
	Payload       json.RawMessage `json:"payload"`
	Related       []string        `json:"related,omitempty"`
	Evidence      []EvidenceRef   `json:"evidence,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

type NewRequest struct {
	RunID         string
	TaskID        string
	ProducerAgent string
	Type          Type
	Status        Status
	Summary       string
	Payload       any
	Related       []string
	Evidence      []EvidenceRef
}

func New(request NewRequest, now time.Time) (Artifact, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	payload, err := json.Marshal(request.Payload)
	if err != nil {
		return Artifact{}, fmt.Errorf("encode artifact payload: %w", err)
	}
	id, err := newID()
	if err != nil {
		return Artifact{}, err
	}
	status := request.Status
	if status == "" {
		status = StatusActive
	}
	artifact := Artifact{
		SchemaVersion: SchemaVersion,
		ID:            id,
		RunID:         strings.TrimSpace(request.RunID),
		TaskID:        strings.TrimSpace(request.TaskID),
		ProducerAgent: strings.TrimSpace(request.ProducerAgent),
		Type:          request.Type,
		Status:        status,
		Summary:       strings.TrimSpace(request.Summary),
		Payload:       payload,
		Related:       sortedUnique(request.Related),
		Evidence:      append([]EvidenceRef(nil), request.Evidence...),
		CreatedAt:     now.UTC(),
		UpdatedAt:     now.UTC(),
	}
	if err := Validate(artifact); err != nil {
		return Artifact{}, err
	}
	return artifact, nil
}

func Validate(value Artifact) error {
	if value.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported artifact schema version: %d", value.SchemaVersion)
	}
	if !safeID(value.ID, "artifact-") {
		return errors.New("artifact id is invalid")
	}
	if strings.TrimSpace(value.RunID) == "" || strings.TrimSpace(value.TaskID) == "" {
		return errors.New("artifact run_id and task_id are required")
	}
	if strings.TrimSpace(value.ProducerAgent) == "" {
		return errors.New("artifact producer_agent is required")
	}
	if !validType(value.Type) {
		return fmt.Errorf("unsupported artifact type: %q", value.Type)
	}
	if !validStatus(value.Status) {
		return fmt.Errorf("unsupported artifact status: %q", value.Status)
	}
	if strings.TrimSpace(value.Summary) == "" {
		return errors.New("artifact summary is required")
	}
	if len(value.Summary) > 1000 {
		return errors.New("artifact summary exceeds 1000 characters")
	}
	if len(value.Payload) == 0 || !json.Valid(value.Payload) {
		return errors.New("artifact payload must be valid JSON")
	}
	if len(value.Payload) > 256*1024 {
		return errors.New("artifact payload exceeds 256 KiB")
	}
	if err := validateTypedPayload(value.Type, value.Payload); err != nil {
		return err
	}
	if value.CreatedAt.IsZero() || value.UpdatedAt.IsZero() || value.UpdatedAt.Before(value.CreatedAt) {
		return errors.New("artifact timestamps are invalid")
	}
	for _, related := range value.Related {
		if !safeID(related, "artifact-") {
			return fmt.Errorf("invalid related artifact id: %q", related)
		}
		if related == value.ID {
			return errors.New("artifact cannot relate to itself")
		}
	}
	for _, evidence := range value.Evidence {
		if strings.TrimSpace(evidence.Kind) == "" || strings.TrimSpace(evidence.Value) == "" {
			return errors.New("artifact evidence kind and value are required")
		}
		if len(evidence.Value) > 2000 {
			return errors.New("artifact evidence value exceeds 2000 characters")
		}
	}
	return nil
}

func validType(value Type) bool {
	switch value {
	case TypeTaskBrief, TypeArchitectureDecision, TypeResearchNote, TypeImplementationPlan,
		TypeChangeProposal, TypeCodePatchReference, TypeTestPlan, TypeVerificationReport,
		TypeReviewReport, TypeRiskAssessment, TypeQuestion, TypeDecision, TypeHandoff, TypeAPIContract:
		return true
	default:
		return false
	}
}

func validStatus(value Status) bool {
	switch value {
	case StatusDraft, StatusActive, StatusSuperseded, StatusAccepted, StatusRejected:
		return true
	default:
		return false
	}
}

func newID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate artifact id: %w", err)
	}
	return "artifact-" + hex.EncodeToString(value[:]), nil
}

func safeID(value, prefix string) bool {
	if !strings.HasPrefix(value, prefix) {
		return false
	}
	for _, r := range strings.TrimPrefix(value, prefix) {
		if !(r >= 'a' && r <= 'f' || r >= '0' && r <= '9') {
			return false
		}
	}
	return len(strings.TrimPrefix(value, prefix)) == 32
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
