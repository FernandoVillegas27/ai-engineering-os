package controlplane

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCompactEventHistoryReconstructsHistoryAndAppendSequence(t *testing.T) {
	eventRoot := t.TempDir()
	events, err := NewEventStore(eventRoot)
	if err != nil { t.Fatal(err) }
	snapshots, err := NewSnapshotStore(t.TempDir())
	if err != nil { t.Fatal(err) }
	archives, err := NewEventArchiveStore(t.TempDir())
	if err != nil { t.Fatal(err) }

	runID := "run-compact"
	appendEvent(t, events, runID, EventRunStarted, "", "")
	appendEvent(t, events, runID, EventTaskStarted, "task-1", "architect")
	snapshot, err := snapshots.Create(context.Background(), events, runID, time.Now().UTC())
	if err != nil { t.Fatal(err) }
	archive, err := archives.Create(context.Background(), events, snapshots, runID, time.Now().UTC())
	if err != nil { t.Fatal(err) }
	appendEvent(t, events, runID, EventTaskCompleted, "task-1", "architect")

	result, err := CompactEventHistory(
		context.Background(), events, snapshots, archives, runID, time.Now().UTC(),
	)
	if err != nil { t.Fatal(err) }
	if result.ThroughSequence != 2 || result.DeletedFiles != 2 || result.AlreadyPresent {
		t.Fatalf("result=%#v", result)
	}
	if result.ArchiveDigest != archive.RecordDigest {
		t.Fatalf("archive digest=%s want=%s", result.ArchiveDigest, archive.RecordDigest)
	}
	if err := snapshots.Verify(context.Background(), events, snapshot); err != nil {
		t.Fatal(err)
	}

	history, err := events.List(runID)
	if err != nil { t.Fatal(err) }
	if len(history) != 3 {
		t.Fatalf("history=%#v", history)
	}
	for i, event := range history {
		if event.Sequence != uint64(i+1) {
			t.Fatalf("event=%#v", event)
		}
	}

	next, err := NewEvent(NewEventRequest{
		RunID: runID,
		Type: EventArtifactCreated,
		Data: map[string]bool{"ok": true},
	}, time.Now().UTC())
	if err != nil { t.Fatal(err) }
	next, err = events.Append(context.Background(), next)
	if err != nil { t.Fatal(err) }
	if next.Sequence != 4 {
		t.Fatalf("sequence=%d want=4", next.Sequence)
	}

	runDir := filepath.Join(eventRoot, runID)
	if _, err := os.Stat(filepath.Join(runDir, compactedHistoryFilename)); err != nil {
		t.Fatal(err)
	}
	for _, entry := range archive.Entries {
		if _, err := os.Stat(filepath.Join(runDir, entry.Filename)); !os.IsNotExist(err) {
			t.Fatalf("expected archived live event removed: %s err=%v", entry.Filename, err)
		}
	}
}

func TestCompactEventHistoryIsIdempotent(t *testing.T) {
	events, _ := NewEventStore(t.TempDir())
	snapshots, _ := NewSnapshotStore(t.TempDir())
	archives, _ := NewEventArchiveStore(t.TempDir())
	runID := "run-compact-idempotent"
	appendEvent(t, events, runID, EventRunStarted, "", "")
	if _, err := snapshots.Create(context.Background(), events, runID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if _, err := archives.Create(context.Background(), events, snapshots, runID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	first, err := CompactEventHistory(context.Background(), events, snapshots, archives, runID, time.Now().UTC())
	if err != nil { t.Fatal(err) }
	if first.DeletedFiles != 1 { t.Fatalf("first=%#v", first) }
	second, err := CompactEventHistory(context.Background(), events, snapshots, archives, runID, time.Now().UTC())
	if err != nil { t.Fatal(err) }
	if !second.AlreadyPresent || second.DeletedFiles != 0 {
		t.Fatalf("second=%#v", second)
	}
}

func TestCompactedHistoryRejectsTamperedMarker(t *testing.T) {
	eventRoot := t.TempDir()
	events, _ := NewEventStore(eventRoot)
	snapshots, _ := NewSnapshotStore(t.TempDir())
	archives, _ := NewEventArchiveStore(t.TempDir())
	runID := "run-compact-tamper"
	appendEvent(t, events, runID, EventRunStarted, "", "")
	if _, err := snapshots.Create(context.Background(), events, runID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if _, err := archives.Create(context.Background(), events, snapshots, runID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if _, err := CompactEventHistory(context.Background(), events, snapshots, archives, runID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(eventRoot, runID, compactedHistoryFilename)
	data, err := os.ReadFile(path)
	if err != nil { t.Fatal(err) }
	if len(data) < 32 { t.Fatal("unexpected compacted history length") }
	data[len(data)/2] ^= 1
	if err := os.WriteFile(path, data, 0o600); err != nil { t.Fatal(err) }
	if _, err := events.List(runID); err == nil {
		t.Fatal("expected tampered compacted history rejection")
	}
}
