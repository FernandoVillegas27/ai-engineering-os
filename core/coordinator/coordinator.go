package coordinator

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/FernandoVillegas27/ai-engineering-os/core/scheduler"
)

type Worker interface {
	Execute(ctx context.Context, task scheduler.Task) (WorkResult, error)
}

type WorkResult struct {
	Summary      string
	ArtifactRefs []string
	MessageRefs  []string
}

type EventType string

const (
	EventRunStarted    EventType = "run.started"
	EventTaskStarted   EventType = "task.started"
	EventTaskCompleted EventType = "task.completed"
	EventTaskFailed    EventType = "task.failed"
	EventRunCompleted  EventType = "run.completed"
	EventRunCancelled  EventType = "run.cancelled"
)

type Event struct {
	Type         EventType
	TaskID       string
	AgentID      string
	Summary      string
	ArtifactRefs []string
	MessageRefs  []string
}

type EventSink interface {
	Record(ctx context.Context, event Event) error
}

type TaskResult struct {
	TaskID       string
	AgentID      string
	Completed    bool
	Error        string
	Summary      string
	ArtifactRefs []string
	MessageRefs  []string
}

type Result struct {
	Completed bool
	Cancelled bool
	Progress  scheduler.Progress
	Tasks     []TaskResult
}

type Config struct {
	MaxConcurrency int
}

type Coordinator struct {
	Config  Config
	Workers map[string]Worker
	Events  EventSink
}

type completion struct {
	task   scheduler.Task
	result WorkResult
	err    error
}

func (c Coordinator) Run(ctx context.Context, graph *scheduler.Scheduler) (Result, error) {
	if graph == nil {
		return Result{}, errors.New("scheduler is required")
	}
	maxConcurrency := c.Config.MaxConcurrency
	if maxConcurrency <= 0 {
		maxConcurrency = 1
	}
	if len(c.Workers) == 0 {
		return Result{}, errors.New("at least one worker is required")
	}
	if err := c.emit(ctx, Event{Type: EventRunStarted}); err != nil {
		return Result{}, err
	}

	completions := make(chan completion, maxConcurrency)
	active := 0
	var taskResults []TaskResult

	for {
		if err := ctx.Err(); err != nil {
			graph.CancelAll()
			_ = c.emit(context.WithoutCancel(ctx), Event{Type: EventRunCancelled, Summary: err.Error()})
			return Result{
				Cancelled: true,
				Progress:  graph.Progress(),
				Tasks:     sortedTaskResults(taskResults),
			}, fmt.Errorf("coordinator canceled: %w", err)
		}

		capacity := maxConcurrency - active
		if capacity > 0 {
			for _, task := range graph.AcquireReady(capacity) {
				worker, ok := c.Workers[task.AgentID]
				if !ok || worker == nil {
					if err := graph.Fail(task.ID); err != nil {
						return Result{}, err
					}
					message := fmt.Sprintf("no worker registered for agent %s", task.AgentID)
					taskResults = append(taskResults, TaskResult{
						TaskID: task.ID,
						AgentID: task.AgentID,
						Error:  message,
					})
					if err := c.emit(ctx, Event{Type: EventTaskFailed, TaskID: task.ID, AgentID: task.AgentID, Summary: message}); err != nil {
						return Result{}, err
					}
					continue
				}

				if err := c.emit(ctx, Event{Type: EventTaskStarted, TaskID: task.ID, AgentID: task.AgentID}); err != nil {
					graph.CancelAll()
					return Result{}, err
				}
				active++
				go func(task scheduler.Task, worker Worker) {
					result, err := worker.Execute(ctx, task)
					completions <- completion{task: task, result: result, err: err}
				}(task, worker)
			}
		}

		if graph.Done() && active == 0 {
			result := Result{
				Completed: true,
				Progress:  graph.Progress(),
				Tasks:     sortedTaskResults(taskResults),
			}
			if err := c.emit(ctx, Event{Type: EventRunCompleted}); err != nil {
				return result, err
			}
			return result, nil
		}

		if active == 0 {
			// A valid DAG with no running work must either expose a ready task or
			// be terminal. Anything else is an internal scheduler/coordinator bug.
			return Result{
				Progress: graph.Progress(),
				Tasks:    sortedTaskResults(taskResults),
			}, errors.New("coordinator made no progress")
		}

		select {
		case <-ctx.Done():
			continue
		case completed := <-completions:
			active--
			taskResult := TaskResult{
				TaskID:       completed.task.ID,
				AgentID:      completed.task.AgentID,
				Summary:      completed.result.Summary,
				ArtifactRefs: append([]string(nil), completed.result.ArtifactRefs...),
				MessageRefs:  append([]string(nil), completed.result.MessageRefs...),
			}
			if completed.err != nil {
				taskResult.Error = completed.err.Error()
				if err := graph.Fail(completed.task.ID); err != nil {
					return Result{}, err
				}
				if err := c.emit(ctx, Event{
					Type:         EventTaskFailed,
					TaskID:       completed.task.ID,
					AgentID:      completed.task.AgentID,
					Summary:      completed.err.Error(),
					ArtifactRefs: completed.result.ArtifactRefs,
					MessageRefs:  completed.result.MessageRefs,
				}); err != nil {
					graph.CancelAll()
					return Result{}, err
				}
			} else {
				taskResult.Completed = true
				if err := graph.Complete(completed.task.ID); err != nil {
					return Result{}, err
				}
				if err := c.emit(ctx, Event{
					Type:         EventTaskCompleted,
					TaskID:       completed.task.ID,
					AgentID:      completed.task.AgentID,
					Summary:      completed.result.Summary,
					ArtifactRefs: completed.result.ArtifactRefs,
					MessageRefs:  completed.result.MessageRefs,
				}); err != nil {
					graph.CancelAll()
					return Result{}, err
				}
			}
			taskResults = append(taskResults, taskResult)
		}
	}
}

func (c Coordinator) emit(ctx context.Context, event Event) error {
	if c.Events == nil {
		return nil
	}
	if err := c.Events.Record(ctx, event); err != nil {
		return fmt.Errorf("record coordinator event %s: %w", event.Type, err)
	}
	return nil
}

func sortedTaskResults(values []TaskResult) []TaskResult {
	out := append([]TaskResult(nil), values...)
	for i := range out {
		out[i].ArtifactRefs = sortedUnique(out[i].ArtifactRefs)
		out[i].MessageRefs = sortedUnique(out[i].MessageRefs)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TaskID < out[j].TaskID })
	return out
}

func sortedUnique(values []string) []string {
	seen := map[string]bool{}
	for _, value := range values {
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

// MemoryEventSink is useful for tests and embedding. Durable event persistence
// belongs to the Control Plane event-stream phase.
type MemoryEventSink struct {
	mu     sync.Mutex
	Events []Event
}

func (s *MemoryEventSink) Record(_ context.Context, event Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	copyEvent := event
	copyEvent.ArtifactRefs = append([]string(nil), event.ArtifactRefs...)
	copyEvent.MessageRefs = append([]string(nil), event.MessageRefs...)
	s.Events = append(s.Events, copyEvent)
	return nil
}

func (s *MemoryEventSink) Snapshot() []Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Event, len(s.Events))
	copy(out, s.Events)
	return out
}
