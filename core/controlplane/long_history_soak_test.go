package controlplane

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"
)

func TestLongHistorySnapshotArchiveCompactionSoak(t *testing.T) {
	if testing.Short() {
		t.Skip("soak-style long history test")
	}

	events, err := NewEventStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	snapshots, err := NewSnapshotStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	archives, err := NewEventArchiveStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	const runID = "run-long-history-soak"
	appendEvent(t, events, runID, EventRunStarted, "", "")

	var records []Snapshot
	for generation := 1; generation <= 4; generation++ {
		target := uint64(generation * 40)
		for {
			history, err := events.List(runID)
			if err != nil {
				t.Fatal(err)
			}
			if uint64(len(history)) >= target {
				break
			}
			value, err := NewEvent(NewEventRequest{
				RunID:   runID,
				Type:    EventArtifactCreated,
				Summary: fmt.Sprintf("artifact-%d", len(history)+1),
				Data:    map[string]any{"generation": generation},
			}, time.Now().UTC())
			if err != nil {
				t.Fatal(err)
			}
			if _, err := events.Append(context.Background(), value); err != nil {
				t.Fatal(err)
			}
		}

		record, err := snapshots.Create(context.Background(), events, runID, time.Now().UTC())
		if err != nil {
			t.Fatal(err)
		}
		records = append(records, record)

		archive, err := archives.Create(context.Background(), events, snapshots, runID, time.Now().UTC())
		if err != nil {
			t.Fatal(err)
		}
		if archive.ThroughSequence != target {
			t.Fatalf("archive generation %d through=%d want=%d", generation, archive.ThroughSequence, target)
		}

		result, err := CompactEventHistory(
			context.Background(), events, snapshots, archives, runID, time.Now().UTC(),
		)
		if err != nil {
			t.Fatal(err)
		}
		if result.ThroughSequence != target {
			t.Fatalf("compaction generation %d through=%d want=%d", generation, result.ThroughSequence, target)
		}

		history, err := events.List(runID)
		if err != nil {
			t.Fatal(err)
		}
		if uint64(len(history)) != target {
			t.Fatalf("history generation %d len=%d want=%d", generation, len(history), target)
		}
		for i, event := range history {
			if event.Sequence != uint64(i+1) {
				t.Fatalf("generation %d event sequence=%d index=%d", generation, event.Sequence, i)
			}
		}
	}

	// Every earlier snapshot remains independently verifiable after newer
	// compaction generations replace the live historical prefix.
	for _, record := range records {
		if err := snapshots.Verify(context.Background(), events, record); err != nil {
			t.Fatalf("verify snapshot through %d: %v", record.ThroughSequence, err)
		}
	}

	// Add a live suffix after the latest compacted generation.
	for i := 0; i < 25; i++ {
		value, err := NewEvent(NewEventRequest{
			RunID:   runID,
			Type:    EventArtifactCreated,
			Summary: fmt.Sprintf("suffix-%d", i),
			Data:    map[string]int{"suffix": i},
		}, time.Now().UTC())
		if err != nil {
			t.Fatal(err)
		}
		appended, err := events.Append(context.Background(), value)
		if err != nil {
			t.Fatal(err)
		}
		want := uint64(161 + i)
		if appended.Sequence != want {
			t.Fatalf("suffix sequence=%d want=%d", appended.Sequence, want)
		}
	}

	history, err := events.List(runID)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 185 {
		t.Fatalf("history len=%d want=185", len(history))
	}
	full, err := Project(history)
	if err != nil {
		t.Fatal(err)
	}
	projected, usedSnapshot, err := snapshots.ProjectVerified(context.Background(), events, runID)
	if err != nil {
		t.Fatal(err)
	}
	if !usedSnapshot {
		t.Fatal("expected verified snapshot projection")
	}
	if !reflect.DeepEqual(full, projected) {
		t.Fatalf("full and snapshot projection diverged:\nfull=%#v\nprojected=%#v", full, projected)
	}
	if full.LastSequence != 185 || full.ArtifactCount != 184 {
		t.Fatalf("final state=%#v", full)
	}
}
