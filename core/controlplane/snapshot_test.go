package controlplane

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSnapshotCreateVerifyAndLatest(t *testing.T) {
	eventStore, err := NewEventStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	snapshotStore, err := NewSnapshotStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runID := "run-snapshot"
	appendEvent(t, eventStore, runID, EventRunStarted, "", "")
	appendEvent(t, eventStore, runID, EventTaskStarted, "task-1", "architect")

	first, err := snapshotStore.Create(context.Background(), eventStore, runID, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if first.ThroughSequence != 2 || first.State.Status != RunRunning {
		t.Fatalf("first=%#v", first)
	}
	if err := snapshotStore.Verify(context.Background(), eventStore, first); err != nil {
		t.Fatal(err)
	}

	appendEvent(t, eventStore, runID, EventTaskCompleted, "task-1", "architect")
	second, err := snapshotStore.Create(context.Background(), eventStore, runID, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if second.ThroughSequence != 3 || second.State.Progress.Completed != 1 {
		t.Fatalf("second=%#v", second)
	}
	if err := snapshotStore.Verify(context.Background(), eventStore, first); err != nil {
		t.Fatalf("older snapshot must remain valid after suffix append: %v", err)
	}
	latest, err := snapshotStore.Latest(runID)
	if err != nil {
		t.Fatal(err)
	}
	if latest.ThroughSequence != second.ThroughSequence || latest.RecordDigest != second.RecordDigest {
		t.Fatalf("latest=%#v second=%#v", latest, second)
	}
}

func TestSnapshotVerifyRejectsModifiedHistoricalEventBytes(t *testing.T) {
	eventRoot := t.TempDir()
	eventStore, err := NewEventStore(eventRoot)
	if err != nil {
		t.Fatal(err)
	}
	snapshotStore, err := NewSnapshotStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runID := "run-modified-prefix"
	appendEvent(t, eventStore, runID, EventRunStarted, "", "")
	appendEvent(t, eventStore, runID, EventTaskStarted, "task-1", "architect")
	record, err := snapshotStore.Create(context.Background(), eventStore, runID, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}

	runDir := filepath.Join(eventRoot, runID)
	entries, err := os.ReadDir(runDir)
	if err != nil {
		t.Fatal(err)
	}
	var firstEvent string
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".json" {
			firstEvent = filepath.Join(runDir, entry.Name())
			break
		}
	}
	if firstEvent == "" {
		t.Fatal("event file not found")
	}
	data, err := os.ReadFile(firstEvent)
	if err != nil {
		t.Fatal(err)
	}
	// Keep the JSON semantically valid but change the exact immutable bytes.
	data = append(data, ' ', '\n')
	if err := os.WriteFile(firstEvent, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := snapshotStore.Verify(context.Background(), eventStore, record); err == nil ||
		!strings.Contains(err.Error(), "prefix digest mismatch") {
		t.Fatalf("err=%#v", err)
	}
}

func TestSnapshotVerifyRejectsMissingHistoricalEvent(t *testing.T) {
	eventRoot := t.TempDir()
	eventStore, err := NewEventStore(eventRoot)
	if err != nil {
		t.Fatal(err)
	}
	snapshotStore, err := NewSnapshotStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runID := "run-missing-prefix"
	appendEvent(t, eventStore, runID, EventRunStarted, "", "")
	appendEvent(t, eventStore, runID, EventTaskStarted, "task-1", "architect")
	record, err := snapshotStore.Create(context.Background(), eventStore, runID, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(eventRoot, runID))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".json" {
			if err := os.Remove(filepath.Join(eventRoot, runID, entry.Name())); err != nil {
				t.Fatal(err)
			}
			break
		}
	}
	if err := snapshotStore.Verify(context.Background(), eventStore, record); err == nil {
		t.Fatal("expected incomplete historical prefix rejection")
	}
}

func TestSnapshotLoadRejectsTamperedRecord(t *testing.T) {
	eventStore, err := NewEventStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	snapshotRoot := t.TempDir()
	snapshotStore, err := NewSnapshotStore(snapshotRoot)
	if err != nil {
		t.Fatal(err)
	}
	runID := "run-tampered-snapshot"
	appendEvent(t, eventStore, runID, EventRunStarted, "", "")
	appendEvent(t, eventStore, runID, EventTaskStarted, "task-1", "architect")
	record, err := snapshotStore.Create(context.Background(), eventStore, runID, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	path := snapshotStore.snapshotPath(runID, record.ThroughSequence)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	state := raw["state"].(map[string]any)
	state["message_count"] = float64(999)
	modified, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(modified, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := snapshotStore.Load(runID, record.ThroughSequence); err == nil ||
		!strings.Contains(err.Error(), "record digest mismatch") {
		t.Fatalf("err=%#v", err)
	}
}

func TestSnapshotCreateRejectsCorruptEventLog(t *testing.T) {
	eventRoot := t.TempDir()
	eventStore, err := NewEventStore(eventRoot)
	if err != nil {
		t.Fatal(err)
	}
	snapshotStore, err := NewSnapshotStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runID := "run-corrupt-before-snapshot"
	appendEvent(t, eventStore, runID, EventRunStarted, "", "")
	appendEvent(t, eventStore, runID, EventTaskStarted, "task-1", "architect")

	runDir := filepath.Join(eventRoot, runID)
	entries, err := os.ReadDir(runDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "00000000000000000002-") {
			if err := os.WriteFile(filepath.Join(runDir, entry.Name()), []byte("{bad json"), 0o600); err != nil {
				t.Fatal(err)
			}
			break
		}
	}
	if _, err := snapshotStore.Create(context.Background(), eventStore, runID, time.Now().UTC()); err == nil {
		t.Fatal("expected corrupt event log rejection")
	}
}

func TestSnapshotStoreRejectsDuplicateAndSymlinkEntries(t *testing.T) {
	eventStore, err := NewEventStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	snapshotRoot := t.TempDir()
	snapshotStore, err := NewSnapshotStore(snapshotRoot)
	if err != nil {
		t.Fatal(err)
	}
	runID := "run-snapshot-entry"
	appendEvent(t, eventStore, runID, EventRunStarted, "", "")
	record, err := snapshotStore.Create(context.Background(), eventStore, runID, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := snapshotStore.Create(context.Background(), eventStore, runID, time.Now().UTC()); err == nil {
		t.Fatal("expected duplicate immutable snapshot rejection")
	}

	target := filepath.Join(snapshotRoot, "target")
	if err := os.WriteFile(target, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := snapshotStore.snapshotPath(runID, record.ThroughSequence+1)
	if err := os.Symlink(target, link); err != nil {
		if os.IsPermission(err) {
			t.Skip("symlink creation not permitted")
		}
		t.Fatal(err)
	}
	if _, err := snapshotStore.Latest(runID); err == nil {
		t.Fatal("expected symlink snapshot entry rejection")
	}
}
