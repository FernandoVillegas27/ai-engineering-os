package controlplane

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

type ObservabilitySnapshot struct {
	GeneratedAt      time.Time         `json:"generated_at"`
	RunCount         int               `json:"run_count"`
	Runs             map[string]int    `json:"runs"`
	EventCount       int               `json:"event_count"`
	TaskCount        int               `json:"task_count"`
	Tasks            map[string]int    `json:"tasks"`
	AgentCount       int               `json:"agent_count"`
	MessageCount     int               `json:"message_count"`
	ArtifactCount    int               `json:"artifact_count"`
	PendingApprovals int               `json:"pending_approvals"`
	PauseRequested   int               `json:"pause_requested"`
	CancelRequested  int               `json:"cancel_requested"`
	NewestEventAt    time.Time         `json:"newest_event_at,omitempty"`
}

func (s HTTPServer) observability(ctx context.Context, now time.Time) (ObservabilitySnapshot, error) {
	if s.Store == nil {
		return ObservabilitySnapshot{}, fmt.Errorf("control-plane event store is not configured")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	result := ObservabilitySnapshot{
		GeneratedAt: now.UTC(),
		Runs: map[string]int{
			string(RunUnknown): 0,
			string(RunRunning): 0,
			string(RunPaused): 0,
			string(RunCompleted): 0,
			string(RunFailed): 0,
			string(RunCancelled): 0,
		},
		Tasks: map[string]int{
			string(TaskQueued): 0,
			string(TaskAssigned): 0,
			string(TaskRunning): 0,
			string(TaskWaiting): 0,
			string(TaskBlocked): 0,
			string(TaskCompleted): 0,
			string(TaskFailed): 0,
			string(TaskCancelled): 0,
		},
	}

	runIDs, err := s.Store.RunIDs()
	if err != nil {
		return ObservabilitySnapshot{}, err
	}
	for _, runID := range runIDs {
		events, err := s.Store.List(runID)
		if err != nil {
			return ObservabilitySnapshot{}, err
		}
		if len(events) == 0 {
			continue
		}
		state, _, err := s.projectState(ctx, runID)
		if err != nil {
			return ObservabilitySnapshot{}, err
		}
		result.RunCount++
		result.Runs[string(state.Status)]++
		result.EventCount += len(events)
		result.AgentCount += len(state.Agents)
		result.MessageCount += state.MessageCount
		result.ArtifactCount += state.ArtifactCount
		result.PendingApprovals += state.PendingApprovals
		if state.PauseRequested {
			result.PauseRequested++
		}
		if state.CancelRequested {
			result.CancelRequested++
		}
		for _, task := range state.Tasks {
			result.TaskCount++
			result.Tasks[string(task.Status)]++
		}
		last := events[len(events)-1].CreatedAt
		if result.NewestEventAt.IsZero() || last.After(result.NewestEventAt) {
			result.NewestEventAt = last
		}
	}
	return result, nil
}

func (value ObservabilitySnapshot) Prometheus() string {
	var b strings.Builder
	writeMetricHelp(&b, "ai_engineering_os_runs", "Current durable runs by projected status.")
	writeMetricType(&b, "ai_engineering_os_runs", "gauge")
	for _, status := range sortedMetricKeys(value.Runs) {
		fmt.Fprintf(&b, "ai_engineering_os_runs{status=%q} %d\n", status, value.Runs[status])
	}

	writeMetricHelp(&b, "ai_engineering_os_events_total", "Total durable Control Plane events across retained run history.")
	writeMetricType(&b, "ai_engineering_os_events_total", "counter")
	fmt.Fprintf(&b, "ai_engineering_os_events_total %d\n", value.EventCount)

	writeMetricHelp(&b, "ai_engineering_os_tasks", "Current projected tasks by status.")
	writeMetricType(&b, "ai_engineering_os_tasks", "gauge")
	for _, status := range sortedMetricKeys(value.Tasks) {
		fmt.Fprintf(&b, "ai_engineering_os_tasks{status=%q} %d\n", status, value.Tasks[status])
	}

	for _, metric := range []struct {
		name string
		help string
		value int
	}{
		{"ai_engineering_os_agents", "Projected agents across all durable runs.", value.AgentCount},
		{"ai_engineering_os_messages_total", "Projected durable collaboration messages across all runs.", value.MessageCount},
		{"ai_engineering_os_artifacts_total", "Projected durable artifacts across all runs.", value.ArtifactCount},
		{"ai_engineering_os_pending_approvals", "Current pending human approvals across all runs.", value.PendingApprovals},
		{"ai_engineering_os_pause_requested_runs", "Runs with a durable pause request.", value.PauseRequested},
		{"ai_engineering_os_cancel_requested_runs", "Runs with a durable cancel request.", value.CancelRequested},
	} {
		writeMetricHelp(&b, metric.name, metric.help)
		kind := "gauge"
		if strings.HasSuffix(metric.name, "_total") {
			kind = "counter"
		}
		writeMetricType(&b, metric.name, kind)
		fmt.Fprintf(&b, "%s %d\n", metric.name, metric.value)
	}

	writeMetricHelp(&b, "ai_engineering_os_newest_event_timestamp_seconds", "Unix timestamp of the newest durable Control Plane event.")
	writeMetricType(&b, "ai_engineering_os_newest_event_timestamp_seconds", "gauge")
	timestamp := int64(0)
	if !value.NewestEventAt.IsZero() {
		timestamp = value.NewestEventAt.Unix()
	}
	fmt.Fprintf(&b, "ai_engineering_os_newest_event_timestamp_seconds %d\n", timestamp)
	return b.String()
}

func sortedMetricKeys(values map[string]int) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func writeMetricHelp(b *strings.Builder, name, help string) {
	fmt.Fprintf(b, "# HELP %s %s\n", name, help)
}

func writeMetricType(b *strings.Builder, name, kind string) {
	fmt.Fprintf(b, "# TYPE %s %s\n", name, kind)
}
