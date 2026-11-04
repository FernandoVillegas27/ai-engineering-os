package runcontrol

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCancelRequestRoundTripAndIdempotency(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 4, 23, 30, 0, 0, time.UTC)
	first, err := store.RequestCancel("run-123", now)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.RequestCancel("run-123", now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if first.RequestedAt != second.RequestedAt {
		t.Fatalf("cancel request was not idempotent: first=%#v second=%#v", first, second)
	}
	requested, err := store.CancelRequested("run-123")
	if err != nil || !requested {
		t.Fatalf("requested=%t err=%v", requested, err)
	}
	if err := store.Clear("run-123"); err != nil {
		t.Fatal(err)
	}
	requested, err = store.CancelRequested("run-123")
	if err != nil || requested {
		t.Fatalf("requested=%t err=%v", requested, err)
	}
}

func TestCancellationWatcherCancelsContextAfterRequest(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx, stop, err := WithCancellationWatcher(context.Background(), store, "run-watch", 10*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	if _, err := store.RequestCancel("run-watch", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("watcher did not cancel context")
	}
}

func TestCancellationWatcherStartsCancelledWhenRequestAlreadyExists(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.RequestCancel("run-existing", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	ctx, stop, err := WithCancellationWatcher(context.Background(), store, "run-existing", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	select {
	case <-ctx.Done():
	default:
		t.Fatal("context should start cancelled")
	}
}

func TestCancelStoreRejectsUnsafeIDAndSymlinkEntry(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.RequestCancel("../outside", time.Now().UTC()); err == nil {
		t.Fatal("expected unsafe id rejection")
	}
	target := filepath.Join(root, "target")
	if err := os.WriteFile(target, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "run-link.cancel.json")
	if err := os.Symlink(target, link); err != nil {
		if os.IsPermission(err) {
			t.Skip("symlink creation not permitted")
		}
		t.Fatal(err)
	}
	if _, err := store.LoadCancel("run-link"); err == nil {
		t.Fatal("expected symlink cancel request rejection")
	}
}
