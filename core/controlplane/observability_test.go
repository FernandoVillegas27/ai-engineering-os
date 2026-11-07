package controlplane

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestObservabilityAggregatesWithoutRunIdentifiers(t *testing.T) {
	store, err := NewEventStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 7, 3, 0, 0, 0, time.UTC)
	appendValue := func(runID string, typ EventType, taskID, agentID string, at time.Time) {
		t.Helper()
		value, err := NewEvent(NewEventRequest{
			RunID: runID,
			Type: typ,
			TaskID: taskID,
			AgentID: agentID,
			Data: map[string]bool{"ok": true},
		}, at)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.Append(context.Background(), value); err != nil {
			t.Fatal(err)
		}
	}

	appendValue("sensitive-project-run-a", EventRunStarted, "", "", now)
	appendValue("sensitive-project-run-a", EventTaskStarted, "task-a", "agent-a", now.Add(time.Second))
	appendValue("sensitive-project-run-a", EventTaskCompleted, "task-a", "agent-a", now.Add(2*time.Second))
	appendValue("sensitive-project-run-a", EventRunCompleted, "", "", now.Add(3*time.Second))

	appendValue("run-b", EventRunStarted, "", "", now.Add(4*time.Second))
	appendValue("run-b", EventTaskStarted, "task-b", "agent-b", now.Add(5*time.Second))

	snapshot, err := (HTTPServer{Store: store}).observability(context.Background(), now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.RunCount != 2 || snapshot.EventCount != 6 || snapshot.TaskCount != 2 || snapshot.AgentCount != 2 {
		t.Fatalf("snapshot=%#v", snapshot)
	}
	if snapshot.Runs[string(RunCompleted)] != 1 || snapshot.Runs[string(RunRunning)] != 1 {
		t.Fatalf("runs=%#v", snapshot.Runs)
	}
	if snapshot.Tasks[string(TaskCompleted)] != 1 || snapshot.Tasks[string(TaskRunning)] != 1 {
		t.Fatalf("tasks=%#v", snapshot.Tasks)
	}
	if !snapshot.NewestEventAt.Equal(now.Add(5*time.Second)) {
		t.Fatalf("newest=%s", snapshot.NewestEventAt)
	}

	metrics := snapshot.Prometheus()
	if strings.Contains(metrics, "sensitive-project-run-a") || strings.Contains(metrics, "task-a") || strings.Contains(metrics, "agent-a") {
		t.Fatalf("metrics leaked identifiers: %s", metrics)
	}
	for _, expected := range []string{
		`ai_engineering_os_runs{status="completed"} 1`,
		`ai_engineering_os_runs{status="running"} 1`,
		`ai_engineering_os_tasks{status="completed"} 1`,
		`ai_engineering_os_tasks{status="running"} 1`,
		"ai_engineering_os_events_total 6",
	} {
		if !strings.Contains(metrics, expected) {
			t.Fatalf("metrics missing %q:\n%s", expected, metrics)
		}
	}
}

func TestObservabilityRequiresEventStore(t *testing.T) {
	_, err := (HTTPServer{}).observability(context.Background(), time.Now())
	if err == nil {
		t.Fatal("expected missing store error")
	}
}
