package toolapproval

import (
	"crypto/rand"
	"crypto/sha256"
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
	RequestDigest string    `json:"request_digest,omitempty"`
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
	RequestDigest string
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
		RequestDigest: normalizeDigest(request.RequestDigest),
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
	if evidence.RequestDigest != "" && !validDigest(evidence.RequestDigest) {
		return errors.New("tool approval request_digest must be a lowercase sha256 hex digest")
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

func GrantsRequest(evidence Evidence, runID, taskID, toolID, operation, requestDigest string, now time.Time) bool {
	requestDigest = normalizeDigest(requestDigest)
	if requestDigest == "" || evidence.RequestDigest == "" || evidence.RequestDigest != requestDigest {
		return false
	}
	return Grants(evidence, runID, taskID, toolID, operation, now)
}

func DigestRequest(payload []byte) string {
	hash := sha256.Sum256(payload)
	return hex.EncodeToString(hash[:])
}

func normalizeDigest(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func validDigest(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil && value == strings.ToLower(value)
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
