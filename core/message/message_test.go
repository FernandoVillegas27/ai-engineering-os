package message

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testMessage(t *testing.T, runID string) Message {
	t.Helper()
	value, err := New(NewRequest{
		RunID:     runID,
		TaskID:    "task-1",
		FromAgent: "backend-engineer",
		To:        Target{Kind: TargetAgent, ID: "frontend-engineer"},
		Kind:      KindHandoff,
		Priority:  PriorityNormal,
		Payload: map[string]any{
			"summary": "API contract ready",
		},
		ArtifactRefs: []string{"artifact-11111111111111111111111111111111"},
	}, time.Date(2026, 11, 4, 8, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestNewMessageRequiresExplicitBroadcastAuthorization(t *testing.T) {
	_, err := New(NewRequest{
		RunID:     "run-1",
		TaskID:    "task-1",
		FromAgent: "orchestrator",
		To:        Target{Kind: TargetBroadcast},
		Kind:      KindStatus,
		Payload:   map[string]string{"state": "paused"},
	}, time.Now().UTC())
	if err == nil {
		t.Fatal("expected unauthorized broadcast rejection")
	}

	value, err := New(NewRequest{
		RunID:          "run-1",
		TaskID:         "task-1",
		FromAgent:      "orchestrator",
		To:             Target{Kind: TargetBroadcast},
		Kind:           KindStatus,
		Payload:        map[string]string{"state": "paused"},
		AllowBroadcast: true,
	}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if value.To.Kind != TargetBroadcast {
		t.Fatalf("message = %#v", value)
	}
}

func TestNewMessageEnforcesHopAndTTL(t *testing.T) {
	now := time.Now().UTC()
	expired := now.Add(-time.Second)
	if _, err := New(NewRequest{
		RunID:     "run-1",
		TaskID:    "task-1",
		FromAgent: "qa-engineer",
		To:        Target{Kind: TargetOrchestrator},
		Kind:      KindBlocker,
		Payload:   map[string]string{"reason": "failed"},
		ExpiresAt: &expired,
	}, now); err == nil {
		t.Fatal("expected invalid expiry rejection")
	}

	if _, err := New(NewRequest{
		RunID:     "run-1",
		TaskID:    "task-1",
		FromAgent: "qa-engineer",
		To:        Target{Kind: TargetOrchestrator},
		Kind:      KindBlocker,
		Payload:   map[string]string{"reason": "failed"},
		Hop:       9,
	}, now); err == nil {
		t.Fatal("expected hop limit rejection")
	}
}

func TestBusAssignsDeterministicPerRunSequence(t *testing.T) {
	bus, err := NewBus(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	first, err := bus.Append(context.Background(), testMessage(t, "run-1"))
	if err != nil {
		t.Fatal(err)
	}
	secondValue := testMessage(t, "run-1")
	secondValue.FromAgent = "frontend-engineer"
	secondValue.To = Target{Kind: TargetAgent, ID: "backend-engineer"}
	secondValue.Kind = KindResponse
	second, err := bus.Append(context.Background(), secondValue)
	if err != nil {
		t.Fatal(err)
	}
	if first.Sequence != 1 || second.Sequence != 2 {
		t.Fatalf("sequences = %d, %d", first.Sequence, second.Sequence)
	}
	values, err := bus.List("run-1", true, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 2 || values[0].ID != first.ID || values[1].ID != second.ID {
		t.Fatalf("values = %#v", values)
	}
}

func TestBusReplyMustExistInSameRun(t *testing.T) {
	bus, err := NewBus(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	first, err := bus.Append(context.Background(), testMessage(t, "run-1"))
	if err != nil {
		t.Fatal(err)
	}
	reply, err := New(NewRequest{
		RunID:     "run-1",
		TaskID:    "task-1",
		FromAgent: "frontend-engineer",
		To:        Target{Kind: TargetAgent, ID: "backend-engineer"},
		Kind:      KindResponse,
		ReplyTo:   first.ID,
		Payload:   map[string]string{"status": "received"},
		Hop:       1,
	}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bus.Append(context.Background(), reply); err != nil {
		t.Fatal(err)
	}

	missing := reply
	missing.ID = "message-22222222222222222222222222222222"
	missing.ReplyTo = "message-33333333333333333333333333333333"
	missing.Sequence = 0
	if _, err := bus.Append(context.Background(), missing); err == nil {
		t.Fatal("expected missing reply target rejection")
	}
}

func TestBusFiltersExpiredMessages(t *testing.T) {
	bus, err := NewBus(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 4, 8, 0, 0, 0, time.UTC)
	expiry := now.Add(time.Minute)
	value, err := New(NewRequest{
		RunID:     "run-ttl",
		TaskID:    "task-1",
		FromAgent: "researcher",
		To:        Target{Kind: TargetOrchestrator},
		Kind:      KindFinding,
		Payload:   map[string]string{"finding": "temporary"},
		ExpiresAt: &expiry,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bus.Append(context.Background(), value); err != nil {
		t.Fatal(err)
	}
	active, err := bus.List("run-ttl", false, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 {
		t.Fatalf("active = %#v", active)
	}
	active, err = bus.List("run-ttl", false, now.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 0 {
		t.Fatalf("expired message returned: %#v", active)
	}
	all, err := bus.List("run-ttl", true, now.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("includeExpired = %#v", all)
	}
}

func TestBusAppendLockHonorsCancellation(t *testing.T) {
	root := t.TempDir()
	bus, err := NewBus(root)
	if err != nil {
		t.Fatal(err)
	}
	runDir, err := bus.ensureRunDir("run-lock")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, ".append.lock"), []byte("held"), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = bus.Append(ctx, testMessage(t, "run-lock"))
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %#v", err)
	}
}

func TestBusRejectsUnsafeRunDirectorySymlink(t *testing.T) {
	root := t.TempDir()
	bus, err := NewBus(root)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "run-link")
	if err := os.Symlink(target, link); err != nil {
		if os.IsPermission(err) {
			t.Skip("symlink creation not permitted")
		}
		t.Fatal(err)
	}
	if _, err := bus.List("run-link", true, time.Time{}); err == nil {
		t.Fatal("expected symlink run directory rejection")
	}
}


func TestBusReclaimsStaleAppendLock(t *testing.T) {
	root := t.TempDir()
	bus, err := NewBus(root)
	if err != nil {
		t.Fatal(err)
	}
	runDir, err := bus.ensureRunDir("run-stale")
	if err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(runDir, ".append.lock")
	if err := os.WriteFile(lockPath, []byte("stale\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-10 * time.Minute)
	if err := os.Chtimes(lockPath, old, old); err != nil {
		t.Fatal(err)
	}

	value := testMessage(t, "run-stale")
	if _, err := bus.Append(context.Background(), value); err != nil {
		t.Fatal(err)
	}
	values, err := bus.List("run-stale", true, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 1 {
		t.Fatalf("values = %#v", values)
	}
}
