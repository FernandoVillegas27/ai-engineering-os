package controlplane

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEventArchiveCreateLoadAndReconstruct(t *testing.T) {
	eventRoot := t.TempDir()
	snapshotRoot := t.TempDir()
	archiveRoot := t.TempDir()
	events, err := NewEventStore(eventRoot)
	if err != nil { t.Fatal(err) }
	snapshots, err := NewSnapshotStore(snapshotRoot)
	if err != nil { t.Fatal(err) }
	archives, err := NewEventArchiveStore(archiveRoot)
	if err != nil { t.Fatal(err) }

	runID := "run-archive"
	appendEvent(t, events, runID, EventRunStarted, "", "")
	appendEvent(t, events, runID, EventTaskStarted, "task-1", "architect")
	snapshot, err := snapshots.Create(context.Background(), events, runID, time.Now().UTC())
	if err != nil { t.Fatal(err) }
	appendEvent(t, events, runID, EventTaskCompleted, "task-1", "architect")

	record, err := archives.Create(context.Background(), events, snapshots, runID, time.Now().UTC())
	if err != nil { t.Fatal(err) }
	if record.ThroughSequence != 2 || len(record.Entries) != 2 {
		t.Fatalf("record=%#v", record)
	}
	if err := VerifyEventArchiveSnapshot(record, snapshot); err != nil {
		t.Fatal(err)
	}
	loaded, err := archives.Latest(runID)
	if err != nil { t.Fatal(err) }
	values, err := EventsFromArchive(loaded)
	if err != nil { t.Fatal(err) }
	if len(values) != 2 || values[0].Sequence != 1 || values[1].Sequence != 2 {
		t.Fatalf("values=%#v", values)
	}
}

func TestEventArchiveRejectsHistoricalPrefixMutation(t *testing.T) {
	eventRoot := t.TempDir()
	events, err := NewEventStore(eventRoot)
	if err != nil { t.Fatal(err) }
	snapshots, err := NewSnapshotStore(t.TempDir())
	if err != nil { t.Fatal(err) }
	archives, err := NewEventArchiveStore(t.TempDir())
	if err != nil { t.Fatal(err) }

	runID := "run-archive-mutation"
	appendEvent(t, events, runID, EventRunStarted, "", "")
	appendEvent(t, events, runID, EventTaskStarted, "task-1", "architect")
	if _, err := snapshots.Create(context.Background(), events, runID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}

	runDir := filepath.Join(eventRoot, runID)
	entries, err := os.ReadDir(runDir)
	if err != nil { t.Fatal(err) }
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "00000000000000000001-") {
			path := filepath.Join(runDir, entry.Name())
			data, err := os.ReadFile(path)
			if err != nil { t.Fatal(err) }
			if err := os.WriteFile(path, append(data, ' '), 0o600); err != nil { t.Fatal(err) }
			break
		}
	}
	if _, err := archives.Create(context.Background(), events, snapshots, runID, time.Now().UTC()); err == nil ||
		!strings.Contains(err.Error(), "prefix digest mismatch") {
		t.Fatalf("err=%#v", err)
	}
}

func TestEventArchiveDetectsTamperedRecord(t *testing.T) {
	eventRoot := t.TempDir()
	events, _ := NewEventStore(eventRoot)
	snapshots, _ := NewSnapshotStore(t.TempDir())
	archiveRoot := t.TempDir()
	archives, _ := NewEventArchiveStore(archiveRoot)

	runID := "run-archive-tamper"
	appendEvent(t, events, runID, EventRunStarted, "", "")
	snapshot, err := snapshots.Create(context.Background(), events, runID, time.Now().UTC())
	if err != nil { t.Fatal(err) }
	record, err := archives.Create(context.Background(), events, snapshots, runID, time.Now().UTC())
	if err != nil { t.Fatal(err) }
	if err := VerifyEventArchiveSnapshot(record, snapshot); err != nil { t.Fatal(err) }

	path := archives.archivePath(runID, record.ThroughSequence)
	data, err := os.ReadFile(path)
	if err != nil { t.Fatal(err) }
	data = []byte(strings.Replace(string(data), record.Entries[0].SHA256, strings.Repeat("0", 64), 1))
	if err := os.WriteFile(path, data, 0o600); err != nil { t.Fatal(err) }
	if _, err := archives.Load(runID, record.ThroughSequence); err == nil {
		t.Fatal("expected tampered event archive rejection")
	}
}
