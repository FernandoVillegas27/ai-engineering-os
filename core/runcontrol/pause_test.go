package runcontrol

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPauseRequestRoundTripAndClear(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	first, err := store.RequestPause("run-123", now)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.RequestPause("run-123", now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if first.RequestedAt != second.RequestedAt {
		t.Fatalf("pause request was not idempotent: first=%#v second=%#v", first, second)
	}
	requested, err := store.PauseRequested("run-123")
	if err != nil || !requested {
		t.Fatalf("requested=%t err=%v", requested, err)
	}
	if err := store.ClearPause("run-123"); err != nil {
		t.Fatal(err)
	}
	requested, err = store.PauseRequested("run-123")
	if err != nil || requested {
		t.Fatalf("requested=%t err=%v", requested, err)
	}
}

func TestPauseStoreRejectsSymlinkEntry(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "target")
	if err := os.WriteFile(target, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "run-link.pause.json")
	if err := os.Symlink(target, link); err != nil {
		if os.IsPermission(err) {
			t.Skip("symlink creation not permitted")
		}
		t.Fatal(err)
	}
	if _, err := store.LoadPause("run-link"); err == nil {
		t.Fatal("expected symlink pause rejection")
	}
}
