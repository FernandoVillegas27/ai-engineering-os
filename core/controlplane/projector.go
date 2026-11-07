package controlplane

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

type RunStatus string

const (
	RunUnknown   RunStatus = "unknown"
	RunRunning   RunStatus = "running"
	RunPaused    RunStatus = "paused"
	RunCompleted RunStatus = "completed"
	RunFailed    RunStatus = "failed"
	RunCancelled RunStatus = "cancelled"
)

type TaskStatus string

const (
	TaskQueued    TaskStatus = "queued"
	TaskAssigned  TaskStatus = "assigned"
	TaskRunning   TaskStatus = "running"
	TaskWaiting   TaskStatus = "waiting"
	TaskBlocked   TaskStatus = "blocked"
	TaskCompleted TaskStatus = "completed"
	TaskFailed    TaskStatus = "failed"
	TaskCancelled TaskStatus = "cancelled"
)

type AgentStatus string

const (
	AgentIdle      AgentStatus = "idle"
	AgentRunning   AgentStatus = "running"
	AgentWaiting   AgentStatus = "waiting"
	AgentBlocked   AgentStatus = "blocked"
	AgentCompleted AgentStatus = "completed"
	AgentFailed    AgentStatus = "failed"
	AgentCancelled AgentStatus = "cancelled"
)

type TaskState struct {
	ID           string     `json:"id"`
	AgentID      string     `json:"agent_id,omitempty"`
	Status       TaskStatus `json:"status"`
	Summary      string     `json:"summary,omitempty"`
	DependsOn    []string   `json:"depends_on,omitempty"`
	Mutating     bool       `json:"mutating"`
	Scopes         []string              `json:"scopes,omitempty"`
	ToolOperations map[string][]string    `json:"tool_operations,omitempty"`
	LastSequence   uint64                `json:"last_sequence"`
}

type TaskEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type AgentState struct {
	ID          string      `json:"id"`
	Status      AgentStatus `json:"status"`
	ActiveTasks []string    `json:"active_tasks,omitempty"`
	TaskCount   int         `json:"task_count"`
}

type QualityState struct {
	Verification       string `json:"verification,omitempty"`
	Review             string `json:"review,omitempty"`
	BaselineComparisons int   `json:"baseline_comparisons"`
	Corrections        int    `json:"corrections"`
}

type PipelineState struct {
	Proposal         string `json:"proposal,omitempty"`
	Stage            string `json:"stage,omitempty"`
	Integration      string `json:"integration,omitempty"`
	CheckpointPhase  string `json:"checkpoint_phase,omitempty"`
}

type ToolApprovalState struct {
	TaskID       string `json:"task_id"`
	ToolID       string `json:"tool_id"`
	Operation    string `json:"operation"`
	RequestDigest string `json:"request_digest,omitempty"`
	Status       string `json:"status"`
	EvidenceID   string `json:"evidence_id,omitempty"`
	ExpiresAt    string `json:"expires_at,omitempty"`
	LastSequence uint64 `json:"last_sequence"`
}


type Progress struct {
	Total     int `json:"total"`
	Queued    int `json:"queued"`
	Assigned  int `json:"assigned"`
	Running   int `json:"running"`
	Waiting   int `json:"waiting"`
	Blocked   int `json:"blocked"`
	Completed int `json:"completed"`
	Failed    int `json:"failed"`
	Cancelled int `json:"cancelled"`
}

type State struct {
	RunID            string                `json:"run_id"`
	ParentRunID      string                `json:"parent_run_id,omitempty"`
	RetryOfTaskID    string                `json:"retry_of_task_id,omitempty"`
	Status           RunStatus             `json:"status"`
	LastSequence     uint64                `json:"last_sequence"`
	Tasks            map[string]TaskState  `json:"tasks"`
	Agents           map[string]AgentState `json:"agents"`
	Edges            []TaskEdge            `json:"edges,omitempty"`
	Progress         Progress              `json:"progress"`
	Quality          QualityState          `json:"quality"`
	Pipeline         PipelineState         `json:"pipeline"`
	MessageCount     int                   `json:"message_count"`
	ArtifactCount    int                   `json:"artifact_count"`
	PendingApprovals int                   `json:"pending_approvals"`
	ToolApprovals    []ToolApprovalState   `json:"tool_approvals,omitempty"`
	CancelRequested  bool                  `json:"cancel_requested"`
	PauseRequested   bool                  `json:"pause_requested"`
	Runtime          string                `json:"runtime,omitempty"`
	Model            string                `json:"model,omitempty"`
}

func Project(events []Event) (State, error) {
	if len(events) == 0 {
		return State{}, errors.New("control-plane projection requires at least one event")
	}
	runID := events[0].RunID
	state := State{
		RunID:  runID,
		Status: RunUnknown,
		Tasks:  map[string]TaskState{},
		Agents: map[string]AgentState{},
	}

	for i, event := range events {
		expected := uint64(i + 1)
		if event.Sequence != expected {
			return State{}, fmt.Errorf("control-plane projection sequence mismatch: got %d want %d", event.Sequence, expected)
		}
		if event.RunID != runID {
			return State{}, errors.New("control-plane projection cannot mix run ids")
		}
		if err := ValidateEvent(event); err != nil {
			return State{}, fmt.Errorf("invalid control-plane event %d: %w", event.Sequence, err)
		}
		if event.Sequence == 1 && event.Type != EventRunStarted {
			return State{}, errors.New("first control-plane event must be run.started")
		}
		if terminalRun(state.Status) && !allowedAfterTerminal(event.Type) {
			return State{}, fmt.Errorf("event %s appears after terminal run state", event.Type)
		}

		if err := applyProjectionEvent(&state, event); err != nil {
			return State{}, err
		}
		state.LastSequence = event.Sequence
	}
	state.recompute()
	return state, nil
}

func applyProjectionEvent(state *State, event Event) error {
	switch event.Type {
	case EventRunStarted:
		state.Status = RunRunning
		var metadata struct {
		ParentRunID   string `json:"parent_run_id"`
		RetryOfTaskID string `json:"retry_of_task_id"`
		}
		if err := json.Unmarshal(event.Data, &metadata); err != nil {
		return fmt.Errorf("decode run-start metadata: %w", err)
		}
		state.ParentRunID = metadata.ParentRunID
		state.RetryOfTaskID = metadata.RetryOfTaskID
	case EventRunPauseRequested:
		state.PauseRequested = true
	case EventRunResumed:
		state.Status = RunRunning
		state.PauseRequested = false
	case EventRunPaused:
		state.Status = RunPaused
		state.PauseRequested = false
	case EventRunRecovered:
		var metadata struct {
		Status          string            `json:"status"`
		CheckpointPhase string            `json:"checkpoint_phase"`
		Tasks           map[string]string `json:"tasks"`
		}
		if err := json.Unmarshal(event.Data, &metadata); err != nil {
		return fmt.Errorf("decode run recovery metadata: %w", err)
		}
		for taskID, recoveredStatus := range metadata.Tasks {
		task := state.task(taskID)
		if task.ID == "" {
			return fmt.Errorf("recovery references unknown task %s", taskID)
		}
		switch recoveredStatus {
		case "queued":
			task.Status = TaskQueued
		case "completed":
			task.Status = TaskCompleted
		case "failed":
			task.Status = TaskFailed
		case "blocked":
			task.Status = TaskBlocked
		case "cancelled":
			task.Status = TaskCancelled
		default:
			return fmt.Errorf(
			"unsupported recovered task status for %s: %s",
			taskID,
			recoveredStatus,
			)
		}
		task.LastSequence = event.Sequence
		state.Tasks[taskID] = task
		}
		switch metadata.Status {
		case "paused":
		state.Status = RunPaused
		case "failed":
		state.Status = RunFailed
		default:
		return fmt.Errorf("unsupported recovered run status: %s", metadata.Status)
		}
		state.PauseRequested = false
		state.CancelRequested = false
		if metadata.CheckpointPhase != "" {
		state.Pipeline.CheckpointPhase = metadata.CheckpointPhase
		state.Pipeline.Proposal = "completed"
		state.Pipeline.Stage = "completed"
		state.Pipeline.Integration = "completed"
		state.Quality = QualityState{}
		}
	case EventRunCompleted:
		state.Status = RunCompleted
	case EventRunFailed:
		state.Status = RunFailed
	case EventRunCancelRequested:
		state.CancelRequested = true
	case EventRunCancelled:
		state.Status = RunCancelled
		state.CancelRequested = false
	case EventTaskCreated:
		if _, exists := state.Tasks[event.TaskID]; exists {
		return fmt.Errorf("task %s already exists", event.TaskID)
		}
		var metadata struct {
		DependsOn      []string            `json:"depends_on"`
		Mutating       bool                `json:"mutating"`
		Scopes         []string            `json:"scopes"`
		ToolOperations map[string][]string `json:"tool_operations"`
		}
		if err := json.Unmarshal(event.Data, &metadata); err != nil {
		return fmt.Errorf("decode task metadata for %s: %w", event.TaskID, err)
		}
		state.Tasks[event.TaskID] = TaskState{
		ID:           event.TaskID,
		AgentID:      event.AgentID,
		Status:       TaskQueued,
		Summary:      event.Summary,
		DependsOn:    sortedStrings(metadata.DependsOn),
		Mutating:     metadata.Mutating,
		Scopes:         sortedStrings(metadata.Scopes),
		ToolOperations: normalizedToolOperations(metadata.ToolOperations),
		LastSequence:   event.Sequence,
		}
	case EventTaskAssigned:
		task := state.task(event.TaskID)
		task.AgentID = event.AgentID
		task.Status = TaskAssigned
		task.Summary = event.Summary
		task.LastSequence = event.Sequence
		state.Tasks[event.TaskID] = task
	case EventTaskStarted:
		task := state.task(event.TaskID)
		if task.ID == "" {
		task.ID = event.TaskID
		}
		task.AgentID = event.AgentID
		task.Status = TaskRunning
		task.Summary = event.Summary
		task.LastSequence = event.Sequence
		state.Tasks[event.TaskID] = task
	case EventTaskWaiting:
		task := state.task(event.TaskID)
		task.ID = event.TaskID
		if event.AgentID != "" {
		task.AgentID = event.AgentID
		}
		task.Status = TaskWaiting
		task.Summary = event.Summary
		task.LastSequence = event.Sequence
		state.Tasks[event.TaskID] = task
	case EventTaskBlocked:
		task := state.task(event.TaskID)
		task.ID = event.TaskID
		if event.AgentID != "" {
		task.AgentID = event.AgentID
		}
		task.Status = TaskBlocked
		task.Summary = event.Summary
		task.LastSequence = event.Sequence
		state.Tasks[event.TaskID] = task
	case EventTaskCompleted:
		task := state.task(event.TaskID)
		task.ID = event.TaskID
		task.AgentID = event.AgentID
		task.Status = TaskCompleted
		task.Summary = event.Summary
		task.LastSequence = event.Sequence
		state.Tasks[event.TaskID] = task
	case EventTaskFailed:
		task := state.task(event.TaskID)
		task.ID = event.TaskID
		task.AgentID = event.AgentID
		task.Status = TaskFailed
		task.Summary = event.Summary
		task.LastSequence = event.Sequence
		state.Tasks[event.TaskID] = task
	case EventTaskCancelled:
		task := state.task(event.TaskID)
		task.ID = event.TaskID
		task.Status = TaskCancelled
		task.Summary = event.Summary
		task.LastSequence = event.Sequence
		state.Tasks[event.TaskID] = task
	case EventProposalStarted:
		state.Pipeline.Proposal = "running"
	case EventProposalCompleted:
		state.Pipeline.Proposal = "completed"
	case EventProposalFailed:
		state.Pipeline.Proposal = "failed"
	case EventStageStarted:
		state.Pipeline.Stage = "running"
	case EventStageCompleted:
		state.Pipeline.Stage = "completed"
	case EventStageFailed:
		state.Pipeline.Stage = "failed"
	case EventIntegrationStarted:
		state.Pipeline.Integration = "running"
	case EventIntegrationCompleted:
		state.Pipeline.Integration = "completed"
	case EventIntegrationFailed:
		state.Pipeline.Integration = "failed"
	case EventAgentMessageSent:
		state.MessageCount++
	case EventArtifactCreated:
		state.ArtifactCount++
	case EventCheckpointCreated:
		var metadata struct {
		Phase string `json:"phase"`
		}
		if err := json.Unmarshal(event.Data, &metadata); err != nil {
		return fmt.Errorf("decode checkpoint metadata: %w", err)
		}
		state.Pipeline.CheckpointPhase = metadata.Phase
	case EventVerificationStarted:
		state.Quality.Verification = "running"
	case EventVerificationPassed:
		state.Quality.Verification = "passed"
	case EventVerificationFailed:
		state.Quality.Verification = "failed"
	case EventBaselineCompared:
		state.Quality.BaselineComparisons++
	case EventCorrectionStarted:
		state.Quality.Corrections++
	case EventCorrectionFailed:
		state.Quality.Verification = "failed"
	case EventReviewStarted:
		state.Quality.Review = "running"
	case EventReviewApproved:
		state.Quality.Review = "approved"
	case EventReviewRejected:
		state.Quality.Review = "rejected"
	case EventApprovalRequested:
		state.PendingApprovals++
		if err := applyToolApprovalProjection(state, event, "pending"); err != nil {
			return err
		}
	case EventApprovalGranted:
		if state.PendingApprovals > 0 {
			state.PendingApprovals--
		}
		if err := applyToolApprovalProjection(state, event, "granted"); err != nil {
			return err
		}
	case EventApprovalRejected:
		if state.PendingApprovals > 0 {
			state.PendingApprovals--
		}
		if err := applyToolApprovalProjection(state, event, "rejected"); err != nil {
			return err
		}
	case EventRuntimeSelected:
		state.Runtime = event.Summary
	case EventModelEscalated:
		state.Model = event.Summary
	}
	return nil
}

func (s State) task(id string) TaskState {
	return s.Tasks[id]
}

func applyToolApprovalProjection(state *State, event Event, status string) error {
	var metadata struct {
		Kind       string `json:"kind"`
		ToolID     string `json:"tool_id"`
		Operation  string `json:"operation"`
		RequestDigest string `json:"request_digest"`
		EvidenceID string `json:"evidence_id"`
		ExpiresAt  any    `json:"expires_at"`
	}
	if err := json.Unmarshal(event.Data, &metadata); err != nil {
		return fmt.Errorf("decode approval metadata: %w", err)
	}
	if metadata.Kind != "tool-operation" {
		return nil
	}
	metadata.ToolID = strings.TrimSpace(metadata.ToolID)
	metadata.Operation = strings.TrimSpace(metadata.Operation)
	if strings.TrimSpace(event.TaskID) == "" || metadata.ToolID == "" || metadata.Operation == "" {
		return errors.New("tool-operation approval event requires task_id, tool_id and operation")
	}
	expiresAt := ""
	if metadata.ExpiresAt != nil {
		if raw, err := json.Marshal(metadata.ExpiresAt); err == nil && string(raw) != "null" {
			expiresAt = strings.Trim(string(raw), "\"")
		}
	}
	value := ToolApprovalState{
		TaskID:       event.TaskID,
		ToolID:       metadata.ToolID,
		Operation:    metadata.Operation,
		RequestDigest: strings.TrimSpace(metadata.RequestDigest),
		Status:       status,
		EvidenceID:   strings.TrimSpace(metadata.EvidenceID),
		ExpiresAt:    expiresAt,
		LastSequence: event.Sequence,
	}
	for i := range state.ToolApprovals {
		current := &state.ToolApprovals[i]
		if current.TaskID == value.TaskID &&
			current.ToolID == value.ToolID &&
			current.Operation == value.Operation {
			*current = value
			return nil
		}
	}
	state.ToolApprovals = append(state.ToolApprovals, value)
	return nil
}

func (s *State) recompute() {
	s.Progress = Progress{}
	s.Agents = map[string]AgentState{}
	s.Edges = nil
	for _, task := range s.Tasks {
		s.Progress.Total++
		for _, dependency := range task.DependsOn {
			s.Edges = append(s.Edges, TaskEdge{From: dependency, To: task.ID})
		}
		switch task.Status {
		case TaskQueued:
			s.Progress.Queued++
		case TaskAssigned:
			s.Progress.Assigned++
		case TaskRunning:
			s.Progress.Running++
		case TaskWaiting:
			s.Progress.Waiting++
		case TaskBlocked:
			s.Progress.Blocked++
		case TaskCompleted:
			s.Progress.Completed++
		case TaskFailed:
			s.Progress.Failed++
		case TaskCancelled:
			s.Progress.Cancelled++
		}
		if task.AgentID == "" {
			continue
		}
		agent := s.Agents[task.AgentID]
		agent.ID = task.AgentID
		agent.TaskCount++
		if task.Status == TaskRunning || task.Status == TaskWaiting || task.Status == TaskBlocked {
			agent.ActiveTasks = append(agent.ActiveTasks, task.ID)
		}
		agent.Status = mergeAgentStatus(agent.Status, task.Status)
		s.Agents[task.AgentID] = agent
	}
	for id, agent := range s.Agents {
		sort.Strings(agent.ActiveTasks)
		s.Agents[id] = agent
	}
	sort.Slice(s.ToolApprovals, func(i, j int) bool {
		if s.ToolApprovals[i].TaskID != s.ToolApprovals[j].TaskID {
			return s.ToolApprovals[i].TaskID < s.ToolApprovals[j].TaskID
		}
		if s.ToolApprovals[i].ToolID != s.ToolApprovals[j].ToolID {
			return s.ToolApprovals[i].ToolID < s.ToolApprovals[j].ToolID
		}
		return s.ToolApprovals[i].Operation < s.ToolApprovals[j].Operation
	})
	sort.Slice(s.Edges, func(i, j int) bool {
		if s.Edges[i].From != s.Edges[j].From {
			return s.Edges[i].From < s.Edges[j].From
		}
		return s.Edges[i].To < s.Edges[j].To
	})
}

func sortedStrings(values []string) []string {
	out := append([]string(nil), values...)
	sort.Strings(out)
	return out
}

func mergeAgentStatus(current AgentStatus, task TaskStatus) AgentStatus {
	candidate := AgentIdle
	switch task {
	case TaskRunning:
		candidate = AgentRunning
	case TaskBlocked:
		candidate = AgentBlocked
	case TaskWaiting, TaskAssigned, TaskQueued:
		candidate = AgentWaiting
	case TaskFailed:
		candidate = AgentFailed
	case TaskCancelled:
		candidate = AgentCancelled
	case TaskCompleted:
		candidate = AgentCompleted
	}
	if agentRank(candidate) > agentRank(current) {
		return candidate
	}
	return current
}

func agentRank(status AgentStatus) int {
	switch status {
	case AgentRunning:
		return 7
	case AgentBlocked:
		return 6
	case AgentWaiting:
		return 5
	case AgentFailed:
		return 4
	case AgentCancelled:
		return 3
	case AgentCompleted:
		return 2
	case AgentIdle:
		return 1
	default:
		return 0
	}
}

func terminalRun(status RunStatus) bool {
	return status == RunCompleted || status == RunFailed || status == RunCancelled
}


func allowedAfterTerminal(eventType EventType) bool {
	switch eventType {
	case EventArtifactCreated, EventAgentMessageSent, EventApprovalGranted, EventApprovalRejected:
		return true
	default:
		return false
	}
}


func normalizedToolOperations(values map[string][]string) map[string][]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string][]string, len(values))
	for toolID, operations := range values {
		toolID = strings.TrimSpace(toolID)
		if toolID == "" {
			continue
		}
		normalized := sortedUniqueStrings(operations)
		if len(normalized) == 0 {
			continue
		}
		out[toolID] = normalized
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func sortedUniqueStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
