package controlplane

import (
	"testing"
	"time"
)

func TestProjectorExposesExactToolApprovalLifecycle(t *testing.T) {
	now := time.Date(2026, 11, 7, 19, 0, 0, 0, time.UTC)
	requestDigest := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	makeEvent := func(sequence uint64, typ EventType, taskID string, data any) Event {
		t.Helper()
		event, err := NewEvent(NewEventRequest{
			RunID:  "run-approval",
			Type:   typ,
			TaskID: taskID,
			Data:   data,
		}, now.Add(time.Duration(sequence)*time.Second))
		if err != nil {
			t.Fatal(err)
		}
		event.Sequence = sequence
		return event
	}

	events := []Event{
		makeEvent(1, EventRunStarted, "", map[string]any{"source": "test"}),
		makeEvent(2, EventApprovalRequested, "task-a", map[string]any{
			"kind": "tool-operation", "tool_id": "github", "operation": "create-pull-request",
			"request_digest": requestDigest,
		}),
		makeEvent(3, EventApprovalRequested, "task-a", map[string]any{
			"kind": "tool-operation", "tool_id": "github", "operation": "create-commit",
		}),
		makeEvent(4, EventApprovalGranted, "task-a", map[string]any{
			"kind": "tool-operation", "tool_id": "github", "operation": "create-pull-request",
			"request_digest": requestDigest,
			"evidence_id": "approval-11111111111111111111111111111111",
			"expires_at": "2026-11-07T19:15:00Z",
		}),
	}

	state, err := Project(events)
	if err != nil {
		t.Fatal(err)
	}
	if state.PendingApprovals != 1 {
		t.Fatalf("pending=%d", state.PendingApprovals)
	}
	if len(state.ToolApprovals) != 2 {
		t.Fatalf("approvals=%#v", state.ToolApprovals)
	}
	var granted, pending *ToolApprovalState
	for i := range state.ToolApprovals {
		value := &state.ToolApprovals[i]
		switch value.Operation {
		case "create-pull-request":
			granted = value
		case "create-commit":
			pending = value
		}
	}
	if granted == nil || granted.Status != "granted" ||
		granted.RequestDigest != requestDigest ||
		granted.EvidenceID != "approval-11111111111111111111111111111111" ||
		granted.ExpiresAt != "2026-11-07T19:15:00Z" {
		t.Fatalf("granted=%#v", granted)
	}
	if pending == nil || pending.Status != "pending" || pending.EvidenceID != "" {
		t.Fatalf("pending approval=%#v", pending)
	}
}

func TestProjectorKeepsLegacyApprovalCountWithoutInventingToolAuthority(t *testing.T) {
	now := time.Now().UTC()
	start, err := NewEvent(NewEventRequest{
		RunID: "run-legacy-approval", Type: EventRunStarted, Data: map[string]bool{"ok": true},
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	request, err := NewEvent(NewEventRequest{
		RunID: "run-legacy-approval", Type: EventApprovalRequested,
		Data: map[string]any{"kind": "promotion"},
	}, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	start.Sequence = 1
	request.Sequence = 2

	state, err := Project([]Event{start, request})
	if err != nil {
		t.Fatal(err)
	}
	if state.PendingApprovals != 1 {
		t.Fatalf("pending=%d", state.PendingApprovals)
	}
	if len(state.ToolApprovals) != 0 {
		t.Fatalf("legacy approval invented tool authority: %#v", state.ToolApprovals)
	}
}

func TestProjectorRejectsMalformedToolApprovalMetadata(t *testing.T) {
	now := time.Now().UTC()
	start, _ := NewEvent(NewEventRequest{
		RunID: "run-bad-approval", Type: EventRunStarted, Data: map[string]bool{"ok": true},
	}, now)
	bad, _ := NewEvent(NewEventRequest{
		RunID: "run-bad-approval", Type: EventApprovalRequested, TaskID: "task-a",
		Data: map[string]any{"kind": "tool-operation", "tool_id": "github"},
	}, now.Add(time.Second))
	start.Sequence = 1
	bad.Sequence = 2
	if _, err := Project([]Event{start, bad}); err == nil {
		t.Fatal("expected malformed tool-operation approval rejection")
	}
}
