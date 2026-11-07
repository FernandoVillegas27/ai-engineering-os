package scheduler

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

type Status string

const (
	StatusQueued    Status = "queued"
	StatusReady     Status = "ready"
	StatusRunning   Status = "running"
	StatusWaiting   Status = "waiting"
	StatusBlocked   Status = "blocked"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
)

type Task struct {
	ID             string              `json:"id"`
	AgentID        string              `json:"agent_id"`
	DependsOn      []string            `json:"depends_on,omitempty"`
	Mutating       bool                `json:"mutating"`
	Scopes         []string            `json:"scopes,omitempty"`
	ToolOperations map[string][]string `json:"tool_operations,omitempty"`
	Status         Status              `json:"status"`
}

type Progress struct {
	Total     int `json:"total"`
	Queued    int `json:"queued"`
	Ready     int `json:"ready"`
	Running   int `json:"running"`
	Waiting   int `json:"waiting"`
	Blocked   int `json:"blocked"`
	Completed int `json:"completed"`
	Failed    int `json:"failed"`
	Cancelled int `json:"cancelled"`
}

type Scheduler struct {
	tasks map[string]*Task
	order []string
}

func New(tasks []Task) (*Scheduler, error) {
	if len(tasks) == 0 {
		return nil, errors.New("scheduler requires at least one task")
	}
	s := &Scheduler{tasks: map[string]*Task{}}
	for _, input := range tasks {
		id := strings.TrimSpace(input.ID)
		agent := strings.TrimSpace(input.AgentID)
		if id == "" || agent == "" {
			return nil, errors.New("task id and agent id are required")
		}
		if _, exists := s.tasks[id]; exists {
			return nil, fmt.Errorf("duplicate task id: %s", id)
		}
		normalizedScopes, err := normalizeScopes(input.Scopes)
		if err != nil {
			return nil, fmt.Errorf("task %s: %w", id, err)
		}
		status := input.Status
		if status == "" {
			status = StatusQueued
		}
		if status != StatusQueued && status != StatusCompleted {
			return nil, fmt.Errorf("initial task %s status must be queued or completed", id)
		}
		task := input
		task.ID = id
		task.AgentID = agent
		task.DependsOn = uniqueSorted(input.DependsOn)
		task.Scopes = normalizedScopes
		task.ToolOperations = cloneToolOperations(input.ToolOperations)
		task.Status = status
		if task.Mutating && len(task.Scopes) == 0 {
			return nil, fmt.Errorf("mutating task %s requires at least one file scope", id)
		}
		s.tasks[id] = &task
		s.order = append(s.order, id)
	}
	sort.Strings(s.order)

	for _, id := range s.order {
		task := s.tasks[id]
		for _, dep := range task.DependsOn {
			if dep == id {
				return nil, fmt.Errorf("task %s depends on itself", id)
			}
			if _, ok := s.tasks[dep]; !ok {
				return nil, fmt.Errorf("task %s depends on unknown task %s", id, dep)
			}
		}
	}
	if err := s.validateAcyclic(); err != nil {
		return nil, err
	}
	s.refresh()
	return s, nil
}

func (s *Scheduler) Ready() []Task {
	if s == nil {
		return nil
	}
	s.refresh()
	var out []Task
	for _, id := range s.order {
		if s.tasks[id].Status == StatusReady {
			out = append(out, cloneTask(*s.tasks[id]))
		}
	}
	return out
}

func (s *Scheduler) AcquireReady(limit int) []Task {
	if s == nil {
		return nil
	}
	s.refresh()
	if limit <= 0 {
		limit = len(s.tasks)
	}
	var acquired []Task
	for _, id := range s.order {
		if len(acquired) >= limit {
			break
		}
		task := s.tasks[id]
		if task.Status != StatusReady {
			continue
		}
		if task.Mutating && s.conflictsWithRunning(*task) {
			continue
		}
		task.Status = StatusRunning
		acquired = append(acquired, cloneTask(*task))
	}
	s.refresh()
	return acquired
}

func (s *Scheduler) Complete(id string) error {
	task, err := s.task(id)
	if err != nil {
		return err
	}
	if task.Status != StatusRunning {
		return fmt.Errorf("task %s must be running before completion", id)
	}
	task.Status = StatusCompleted
	s.refresh()
	return nil
}

func (s *Scheduler) Fail(id string) error {
	task, err := s.task(id)
	if err != nil {
		return err
	}
	if task.Status != StatusRunning {
		return fmt.Errorf("task %s must be running before failure", id)
	}
	task.Status = StatusFailed
	s.refresh()
	return nil
}

func (s *Scheduler) Cancel(id string) error {
	task, err := s.task(id)
	if err != nil {
		return err
	}
	switch task.Status {
	case StatusCompleted, StatusFailed, StatusCancelled:
		return fmt.Errorf("task %s is already terminal", id)
	default:
		task.Status = StatusCancelled
		s.refresh()
		return nil
	}
}

func (s *Scheduler) CancelAll() {
	if s == nil {
		return
	}
	for _, task := range s.tasks {
		switch task.Status {
		case StatusCompleted, StatusFailed, StatusCancelled:
		default:
			task.Status = StatusCancelled
		}
	}
}

func (s *Scheduler) Snapshot() []Task {
	if s == nil {
		return nil
	}
	s.refresh()
	out := make([]Task, 0, len(s.order))
	for _, id := range s.order {
		out = append(out, cloneTask(*s.tasks[id]))
	}
	return out
}

func (s *Scheduler) Progress() Progress {
	var progress Progress
	for _, task := range s.Snapshot() {
		progress.Total++
		switch task.Status {
		case StatusQueued:
			progress.Queued++
		case StatusReady:
			progress.Ready++
		case StatusRunning:
			progress.Running++
		case StatusWaiting:
			progress.Waiting++
		case StatusBlocked:
			progress.Blocked++
		case StatusCompleted:
			progress.Completed++
		case StatusFailed:
			progress.Failed++
		case StatusCancelled:
			progress.Cancelled++
		}
	}
	return progress
}

func (s *Scheduler) Done() bool {
	progress := s.Progress()
	return progress.Total > 0 &&
		progress.Completed+progress.Failed+progress.Blocked+progress.Cancelled == progress.Total
}

func (s *Scheduler) task(id string) (*Task, error) {
	if s == nil {
		return nil, errors.New("scheduler is required")
	}
	task, ok := s.tasks[strings.TrimSpace(id)]
	if !ok {
		return nil, fmt.Errorf("unknown task: %s", id)
	}
	return task, nil
}

func (s *Scheduler) refresh() {
	for _, id := range s.order {
		task := s.tasks[id]
		switch task.Status {
		case StatusRunning, StatusCompleted, StatusFailed, StatusCancelled:
			continue
		}

		blocked := false
		waiting := false
		for _, depID := range task.DependsOn {
			dep := s.tasks[depID]
			switch dep.Status {
			case StatusFailed, StatusBlocked, StatusCancelled:
				blocked = true
			case StatusCompleted:
			default:
				waiting = true
			}
		}
		if blocked {
			task.Status = StatusBlocked
			continue
		}
		if waiting {
			task.Status = StatusWaiting
			continue
		}
		if task.Mutating && s.conflictsWithRunning(*task) {
			task.Status = StatusWaiting
			continue
		}
		task.Status = StatusReady
	}
}

func (s *Scheduler) conflictsWithRunning(candidate Task) bool {
	if !candidate.Mutating {
		return false
	}
	for _, other := range s.tasks {
		if other.ID == candidate.ID || other.Status != StatusRunning || !other.Mutating {
			continue
		}
		if scopesOverlap(candidate.Scopes, other.Scopes) {
			return true
		}
	}
	return false
}

func scopesOverlap(a, b []string) bool {
	for _, left := range a {
		for _, right := range b {
			if left == right || strings.HasPrefix(left, right+"/") || strings.HasPrefix(right, left+"/") {
				return true
			}
		}
	}
	return false
}

func normalizeScopes(values []string) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	for _, value := range values {
		value = filepath.ToSlash(strings.TrimSpace(value))
		value = strings.TrimPrefix(value, "./")
		value = strings.TrimSuffix(value, "/")
		clean := filepath.ToSlash(filepath.Clean(value))
		if value == "" || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") || strings.Contains(clean, "*") {
			return nil, fmt.Errorf("invalid file scope %q", value)
		}
		if !seen[clean] {
			seen[clean] = true
			out = append(out, clean)
		}
	}
	sort.Strings(out)
	return out, nil
}

func uniqueSorted(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}

func cloneTask(task Task) Task {
	task.DependsOn = append([]string(nil), task.DependsOn...)
	task.Scopes = append([]string(nil), task.Scopes...)
	task.ToolOperations = cloneToolOperations(task.ToolOperations)
	return task
}

func cloneToolOperations(values map[string][]string) map[string][]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string][]string, len(values))
	for toolID, operations := range values {
		toolID = strings.TrimSpace(toolID)
		if toolID == "" {
			continue
		}
		normalized := uniqueSorted(operations)
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

func (s *Scheduler) validateAcyclic() error {
	const (
		unseen = iota
		visiting
		done
	)
	state := map[string]int{}
	var visit func(string) error
	visit = func(id string) error {
		switch state[id] {
		case visiting:
			return fmt.Errorf("task graph contains a cycle at %s", id)
		case done:
			return nil
		}
		state[id] = visiting
		for _, dep := range s.tasks[id].DependsOn {
			if err := visit(dep); err != nil {
				return err
			}
		}
		state[id] = done
		return nil
	}
	for _, id := range s.order {
		if err := visit(id); err != nil {
			return err
		}
	}
	return nil
}
