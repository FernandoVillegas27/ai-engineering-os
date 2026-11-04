package runstate

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
)

const SchemaVersion = 1

type Status string

const (
	StatusPlanned   Status = "planned"
	StatusRunning   Status = "running"
	StatusPaused    Status = "paused"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
)

type WorkUnitStatus string

const (
	WorkUnitQueued    WorkUnitStatus = "queued"
	WorkUnitReady     WorkUnitStatus = "ready"
	WorkUnitRunning   WorkUnitStatus = "running"
	WorkUnitWaiting   WorkUnitStatus = "waiting"
	WorkUnitBlocked   WorkUnitStatus = "blocked"
	WorkUnitCompleted WorkUnitStatus = "completed"
	WorkUnitFailed    WorkUnitStatus = "failed"
	WorkUnitCancelled WorkUnitStatus = "cancelled"
)

type RuntimeSelection struct {
	Provider    string `json:"provider,omitempty"`
	ModelPolicy string `json:"model_policy,omitempty"`
	Model       string `json:"model,omitempty"`
}

type QualitySnapshot struct {
	Passed             bool   `json:"passed"`
	Reason             string `json:"reason,omitempty"`
	VerificationRuns   int    `json:"verification_runs,omitempty"`
	ReviewRuns         int    `json:"review_runs,omitempty"`
	CorrectionRuns     int    `json:"correction_runs,omitempty"`
	BaselineComparisons int   `json:"baseline_comparisons,omitempty"`
}

type Approval struct {
	Kind      string    `json:"kind"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type WorkUnit struct {
	ID           string                       `json:"id"`
	AgentID      string                       `json:"agent_id"`
	Task         string                       `json:"task"`
	Status       WorkUnitStatus               `json:"status"`
	Capabilities []string                     `json:"capabilities,omitempty"`
	Skills       []string                     `json:"skills,omitempty"`
	Tools        []string                     `json:"tools,omitempty"`
	Permissions  execution.PermissionEnvelope `json:"permissions"`
	DependsOn    []string                     `json:"depends_on,omitempty"`
	ArtifactRefs []string                     `json:"artifact_refs,omitempty"`
	MessageRefs  []string                     `json:"message_refs,omitempty"`
	StartedAt    *time.Time                   `json:"started_at,omitempty"`
	FinishedAt   *time.Time                   `json:"finished_at,omitempty"`
}

type Run struct {
	SchemaVersion int              `json:"schema_version"`
	ID            string           `json:"id"`
	TaskID        string           `json:"task_id"`
	Task          string           `json:"task"`
	ProjectID     string           `json:"project_id"`
	ProjectRoot   string           `json:"project_root,omitempty"`
	Status        Status           `json:"status"`
	Capabilities  []string         `json:"capabilities,omitempty"`
	Runtime       RuntimeSelection `json:"runtime,omitempty"`
	WorkUnits     []WorkUnit       `json:"work_units,omitempty"`
	Quality       *QualitySnapshot `json:"quality,omitempty"`
	Approvals     []Approval       `json:"approvals,omitempty"`
	ArtifactRefs  []string         `json:"artifact_refs,omitempty"`
	MessageRefs   []string         `json:"message_refs,omitempty"`
	CreatedAt     time.Time        `json:"created_at"`
	UpdatedAt     time.Time        `json:"updated_at"`
	FinishedAt    *time.Time       `json:"finished_at,omitempty"`
}

type NewRunRequest struct {
	Task         string
	ProjectID    string
	ProjectRoot  string
	Capabilities []string
	Runtime      RuntimeSelection
	WorkUnits    []execution.WorkUnit
}

func New(request NewRunRequest, now time.Time) (Run, error) {
	if strings.TrimSpace(request.Task) == "" {
		return Run{}, errors.New("run task is required")
	}
	if strings.TrimSpace(request.ProjectID) == "" {
		return Run{}, errors.New("project id is required")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	runID, err := newID("run")
	if err != nil {
		return Run{}, err
	}
	taskID, err := newID("task")
	if err != nil {
		return Run{}, err
	}

	run := Run{
		SchemaVersion: SchemaVersion,
		ID:            runID,
		TaskID:        taskID,
		Task:          strings.TrimSpace(request.Task),
		ProjectID:     strings.TrimSpace(request.ProjectID),
		ProjectRoot:   strings.TrimSpace(request.ProjectRoot),
		Status:        StatusPlanned,
		Capabilities:  sortedUnique(request.Capabilities),
		Runtime:       request.Runtime,
		CreatedAt:     now.UTC(),
		UpdatedAt:     now.UTC(),
	}
	for _, unit := range request.WorkUnits {
		run.WorkUnits = append(run.WorkUnits, WorkUnit{
			ID:           unit.ID,
			AgentID:      unit.AgentID,
			Task:         unit.Task,
			Status:       WorkUnitQueued,
			Capabilities: sortedUnique(unit.Capabilities),
			Skills:       sortedUnique(unit.Skills),
			Tools:        sortedUnique(unit.Tools),
			Permissions:  unit.Permissions,
		})
	}
	if err := Validate(run); err != nil {
		return Run{}, err
	}
	return run, nil
}

func Validate(run Run) error {
	if run.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported run schema version: %d", run.SchemaVersion)
	}
	if strings.TrimSpace(run.ID) == "" || strings.TrimSpace(run.TaskID) == "" {
		return errors.New("run and task ids are required")
	}
	if strings.TrimSpace(run.Task) == "" || strings.TrimSpace(run.ProjectID) == "" {
		return errors.New("run task and project id are required")
	}
	if !validStatus(run.Status) {
		return fmt.Errorf("invalid run status: %q", run.Status)
	}
	if run.CreatedAt.IsZero() || run.UpdatedAt.IsZero() || run.UpdatedAt.Before(run.CreatedAt) {
		return errors.New("invalid run timestamps")
	}
	ids := map[string]bool{}
	for _, unit := range run.WorkUnits {
		if strings.TrimSpace(unit.ID) == "" || strings.TrimSpace(unit.AgentID) == "" {
			return errors.New("work unit id and agent id are required")
		}
		if ids[unit.ID] {
			return fmt.Errorf("duplicate work unit id: %s", unit.ID)
		}
		ids[unit.ID] = true
		if !validWorkUnitStatus(unit.Status) {
			return fmt.Errorf("invalid work unit status for %s: %q", unit.ID, unit.Status)
		}
	}
	for _, unit := range run.WorkUnits {
		for _, dependency := range unit.DependsOn {
			if dependency == unit.ID {
				return fmt.Errorf("work unit %s depends on itself", unit.ID)
			}
			if !ids[dependency] {
				return fmt.Errorf("work unit %s depends on unknown work unit %s", unit.ID, dependency)
			}
		}
	}
	return nil
}

func Transition(run *Run, next Status, now time.Time) error {
	if run == nil {
		return errors.New("run is required")
	}
	if !allowedTransition(run.Status, next) {
		return fmt.Errorf("invalid run transition: %s -> %s", run.Status, next)
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if now.Before(run.UpdatedAt) {
		return errors.New("transition timestamp is before current run state")
	}
	run.Status = next
	run.UpdatedAt = now.UTC()
	if terminal(next) {
		finished := now.UTC()
		run.FinishedAt = &finished
	} else {
		run.FinishedAt = nil
	}
	return Validate(*run)
}

func allowedTransition(current, next Status) bool {
	if current == next {
		return true
	}
	switch current {
	case StatusPlanned:
		return next == StatusRunning || next == StatusCancelled
	case StatusRunning:
		return next == StatusPaused || next == StatusCompleted || next == StatusFailed || next == StatusCancelled
	case StatusPaused:
		return next == StatusRunning || next == StatusCancelled
	default:
		return false
	}
}

func terminal(status Status) bool {
	return status == StatusCompleted || status == StatusFailed || status == StatusCancelled
}

func validStatus(status Status) bool {
	switch status {
	case StatusPlanned, StatusRunning, StatusPaused, StatusCompleted, StatusFailed, StatusCancelled:
		return true
	default:
		return false
	}
}

func validWorkUnitStatus(status WorkUnitStatus) bool {
	switch status {
	case WorkUnitQueued, WorkUnitReady, WorkUnitRunning, WorkUnitWaiting, WorkUnitBlocked, WorkUnitCompleted, WorkUnitFailed, WorkUnitCancelled:
		return true
	default:
		return false
	}
}

func newID(prefix string) (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate %s id: %w", prefix, err)
	}
	return prefix + "-" + hex.EncodeToString(value[:]), nil
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
