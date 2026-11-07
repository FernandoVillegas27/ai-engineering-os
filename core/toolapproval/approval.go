package toolapproval

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

const SchemaVersion = 1

type Status string

const (
	StatusGranted  Status = "granted"
	StatusRejected Status = "rejected"
)

type Evidence struct {
	SchemaVersion int       `json:"schema_version"`
	ID            string    `json:"id"`
	RunID         string    `json:"run_id"`
	TaskID        string    `json:"task_id"`
	ToolID        string    `json:"tool_id"`
	Operation     string    `json:"operation"`
	Status        Status    `json:"status"`
	Actor         string    `json:"actor"`
	CreatedAt     time.Time `json:"created_at"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
}

type Request struct {
	RunID     string
	TaskID    string
	ToolID    string
	Operation string
	Status    Status
	Actor     string
	ExpiresAt *time.Time
}

func New(request Request, now time.Time) (Evidence, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	id, err := newID()
	if err != nil {
		return Evidence{}, err
	}
	evidence := Evidence{
		SchemaVersion: SchemaVersion,
		ID:            id,
		RunID:         strings.TrimSpace(request.RunID),
		TaskID:        strings.TrimSpace(request.TaskID),
		ToolID:        strings.TrimSpace(request.ToolID),
		Operation:     strings.TrimSpace(request.Operation),
		Status:        request.Status,
		Actor:         strings.TrimSpace(request.Actor),
		CreatedAt:     now.UTC(),
	}
	if request.ExpiresAt != nil {
		value := request.ExpiresAt.UTC()
		evidence.ExpiresAt = &value
	}
	if err := Validate(evidence); err != nil {
		return Evidence{}, err
	}
	return evidence, nil
}

func Validate(evidence Evidence) error {
	if evidence.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported tool approval schema version: %d", evidence.SchemaVersion)
	}
	if !safeToken(evidence.ID) || !safeToken(evidence.RunID) || !safeToken(evidence.TaskID) || !safeToken(evidence.ToolID) {
		return errors.New("tool approval contains unsafe identity")
	}
	if strings.TrimSpace(evidence.Operation) == "" {
		return errors.New("tool approval operation is required")
	}
	if evidence.Status != StatusGranted && evidence.Status != StatusRejected {
		return fmt.Errorf("unsupported tool approval status: %q", evidence.Status)
	}
	if evidence.Actor != "human" {
		return errors.New("tool approval actor must be human")
	}
	if evidence.CreatedAt.IsZero() {
		return errors.New("tool approval created_at is required")
	}
	if evidence.ExpiresAt != nil && !evidence.ExpiresAt.After(evidence.CreatedAt) {
		return errors.New("tool approval expiry must be after creation")
	}
	return nil
}

func Grants(evidence Evidence, runID, taskID, toolID, operation string, now time.Time) bool {
	if Validate(evidence) != nil || evidence.Status != StatusGranted {
		return false
	}
	if evidence.RunID != strings.TrimSpace(runID) ||
		evidence.TaskID != strings.TrimSpace(taskID) ||
		evidence.ToolID != strings.TrimSpace(toolID) ||
		evidence.Operation != strings.TrimSpace(operation) {
		return false
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return evidence.ExpiresAt == nil || now.UTC().Before(*evidence.ExpiresAt)
}

func newID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate tool approval id: %w", err)
	}
	return "approval-" + hex.EncodeToString(value[:]), nil
}

func safeToken(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || value == "." || value == ".." || strings.ContainsAny(value, "/\\") {
		return false
	}
	for _, r := range value {
		if !(r == '-' || r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}
