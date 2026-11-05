package collaboration

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/artifact"
	"github.com/FernandoVillegas27/ai-engineering-os/core/coordinator"
	"github.com/FernandoVillegas27/ai-engineering-os/core/message"
	"github.com/FernandoVillegas27/ai-engineering-os/core/scheduler"
)

type HandoffWorker struct {
	Inner         coordinator.Worker
	RunID         string
	ArtifactStore *artifact.Store
	MessageBus    *message.Bus
	Dependents    map[string][]scheduler.Task
	Now           func() time.Time
}

func (w HandoffWorker) Execute(ctx context.Context, task scheduler.Task) (coordinator.WorkResult, error) {
	if w.Inner == nil {
		return coordinator.WorkResult{}, fmt.Errorf("handoff worker requires inner worker")
	}
	if w.ArtifactStore == nil {
		return coordinator.WorkResult{}, fmt.Errorf("handoff worker requires artifact store")
	}
	if w.MessageBus == nil {
		return coordinator.WorkResult{}, fmt.Errorf("handoff worker requires message bus")
	}
	if strings.TrimSpace(w.RunID) == "" {
		return coordinator.WorkResult{}, fmt.Errorf("handoff worker requires run id")
	}

	result, err := w.Inner.Execute(ctx, task)
	if err != nil {
		return result, err
	}

	now := time.Now().UTC()
	if w.Now != nil {
		now = w.Now().UTC()
	}

	artifactValue, err := artifact.New(artifact.NewRequest{
		RunID:         w.RunID,
		TaskID:        task.ID,
		ProducerAgent: task.AgentID,
		Type:          artifact.TypeHandoff,
		Status:        artifact.StatusActive,
		Summary:       handoffSummary(task, result),
		Payload: artifact.HandoffPayload{
			SchemaVersion: artifact.LifecyclePayloadSchemaVersion,
			TaskID:        task.ID,
			AgentID:       task.AgentID,
			Summary:       handoffPayloadSummary(result),
			ArtifactRefs:  result.ArtifactRefs,
			MessageRefs:   result.MessageRefs,
		},
		Related: result.ArtifactRefs,
	}, now)
	if err != nil {
		return result, fmt.Errorf("create handoff artifact: %w", err)
	}
	if err := w.ArtifactStore.Save(artifactValue); err != nil {
		return result, fmt.Errorf("persist handoff artifact: %w", err)
	}

	artifactRefs := append([]string(nil), result.ArtifactRefs...)
	artifactRefs = append(artifactRefs, artifactValue.ID)

	messageRefs := append([]string(nil), result.MessageRefs...)
	dependents := append([]scheduler.Task(nil), w.Dependents[task.ID]...)
	sort.Slice(dependents, func(i, j int) bool {
		if dependents[i].AgentID != dependents[j].AgentID {
			return dependents[i].AgentID < dependents[j].AgentID
		}
		return dependents[i].ID < dependents[j].ID
	})

	for _, dependent := range dependents {
		msg, err := message.New(message.NewRequest{
			RunID:     w.RunID,
			TaskID:    task.ID,
			FromAgent: task.AgentID,
			To: message.Target{
				Kind: message.TargetAgent,
				ID:   dependent.AgentID,
			},
			Kind:         message.KindHandoff,
			Priority:     message.PriorityNormal,
			ArtifactRefs: []string{artifactValue.ID},
			Payload: message.HandoffPayload{
				SchemaVersion: message.PayloadSchemaVersion,
				FromTask:      task.ID,
				ToTask:        dependent.ID,
				Summary:       artifactValue.Summary,
			},
			Hop: 1,
		}, now)
		if err != nil {
			return result, fmt.Errorf("create handoff message to %s: %w", dependent.AgentID, err)
		}
		persisted, err := w.MessageBus.Append(ctx, msg)
		if err != nil {
			return result, fmt.Errorf("persist handoff message to %s: %w", dependent.AgentID, err)
		}
		messageRefs = append(messageRefs, persisted.ID)
	}

	result.ArtifactRefs = uniqueSorted(artifactRefs)
	result.MessageRefs = uniqueSorted(messageRefs)
	return result, nil
}

func BuildDependents(tasks []scheduler.Task) map[string][]scheduler.Task {
	out := map[string][]scheduler.Task{}
	for _, task := range tasks {
		for _, dependency := range task.DependsOn {
			out[dependency] = append(out[dependency], task)
		}
	}
	for id := range out {
		sort.Slice(out[id], func(i, j int) bool {
			if out[id][i].AgentID != out[id][j].AgentID {
				return out[id][i].AgentID < out[id][j].AgentID
			}
			return out[id][i].ID < out[id][j].ID
		})
	}
	return out
}

func handoffPayloadSummary(result coordinator.WorkResult) string {
	summary := strings.TrimSpace(result.Summary)
	if summary == "" {
		return "Task completed without an additional summary."
	}
	return summary
}

func handoffSummary(task scheduler.Task, result coordinator.WorkResult) string {
	summary := strings.TrimSpace(result.Summary)
	if summary == "" {
		summary = "Task completed without an additional summary."
	}
	return fmt.Sprintf("%s completed %s: %s", task.AgentID, task.ID, summary)
}

func uniqueSorted(values []string) []string {
	seen := map[string]bool{}
	var out []string
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
