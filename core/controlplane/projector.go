package controlplane

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
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
	Scopes       []string   `json:"scopes,omitempty"`
	LastSequence uint64     `json:"last_sequence"`
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
	Proposal    string `json:"proposal,omitempty"`
	Stage       string `json:"stage,omitempty"`
	Integration string `json:"integration,omitempty"`
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
	CancelRequested  bool                  `json:"cancel_requested"`
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

		switch event.Type {
		case EventRunStarted:
			state.Status = RunRunning
			var metadata struct {
				ParentRunID   string `json:"parent_run_id"`
				RetryOfTaskID string `json:"retry_of_task_id"`
			}
			if err := json.Unmarshal(event.Data, &metadata); err != nil {
				return State{}, fmt.Errorf("decode run-start metadata: %w", err)
			}
			state.ParentRunID = metadata.ParentRunID
			state.RetryOfTaskID = metadata.RetryOfTaskID
		case EventRunResumed:
			state.Status = RunRunning
		case EventRunPaused:
			state.Status = RunPaused
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
				return State{}, fmt.Errorf("task %s already exists", event.TaskID)
			}
			var metadata struct {
				DependsOn []string `json:"depends_on"`
				Mutating  bool     `json:"mutating"`
				Scopes    []string `json:"scopes"`
			}
			if err := json.Unmarshal(event.Data, &metadata); err != nil {
				return State{}, fmt.Errorf("decode task metadata for %s: %w", event.TaskID, err)
			}
			state.Tasks[event.TaskID] = TaskState{
				ID:           event.TaskID,
				AgentID:      event.AgentID,
				Status:       TaskQueued,
				Summary:      event.Summary,
				DependsOn:    sortedStrings(metadata.DependsOn),
				Mutating:     metadata.Mutating,
				Scopes:       sortedStrings(metadata.Scopes),
				LastSequence: event.Sequence,
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
		case EventApprovalGranted, EventApprovalRejected:
			if state.PendingApprovals > 0 {
				state.PendingApprovals--
			}
		case EventRuntimeSelected:
			state.Runtime = event.Summary
		case EventModelEscalated:
			state.Model = event.Summary
		}
		state.LastSequence = event.Sequence
	}
	state.recompute()
	return state, nil
}

func (s State) task(id string) TaskState {
	return s.Tasks[id]
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
