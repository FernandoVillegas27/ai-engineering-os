package main

import (
	"context"
	"testing"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/controlplane"
)

func TestRunCompactReconstructsHistory(t *testing.T) {
	base := t.TempDir()
	eventsDir := base + "/events"
	snapshotDir := base + "/snapshots"
	archiveDir := base + "/archives"
	events, err := controlplane.NewEventStore(eventsDir)
	if err != nil { t.Fatal(err) }
	runID := "run-cli-compact"
	value, err := controlplane.NewEvent(controlplane.NewEventRequest{
		RunID: runID,
		Type: controlplane.EventRunStarted,
		Data: map[string]bool{"ok": true},
	}, time.Now().UTC())
	if err != nil { t.Fatal(err) }
	if _, err := events.Append(context.Background(), value); err != nil { t.Fatal(err) }
	snapshots, _ := controlplane.NewSnapshotStore(snapshotDir)
	if _, err := snapshots.Create(context.Background(), events, runID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	archives, _ := controlplane.NewEventArchiveStore(archiveDir)
	if _, err := archives.Create(context.Background(), events, snapshots, runID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}

	if err := runRunCompact([]string{
		runID,
		"--events-dir", eventsDir,
		"--snapshot-dir", snapshotDir,
		"--archive-dir", archiveDir,
	}); err != nil {
		t.Fatal(err)
	}
	history, err := events.List(runID)
	if err != nil { t.Fatal(err) }
	if len(history) != 1 || history[0].Sequence != 1 {
		t.Fatalf("history=%#v", history)
	}
}

func TestParseRunCompactArgs(t *testing.T) {
	options, err := parseRunCompactArgs([]string{
		"run-1",
		"--events-dir", "./events",
		"--snapshot-dir", "./snapshots",
		"--archive-dir", "./archives",
		"--json",
	})
	if err != nil { t.Fatal(err) }
	if options.runID != "run-1" ||
		options.eventsDir != "./events" ||
		options.snapshotDir != "./snapshots" ||
		options.archiveDir != "./archives" ||
		!options.json {
		t.Fatalf("options=%#v", options)
	}
	if _, err := parseRunCompactArgs(nil); err == nil {
		t.Fatal("expected missing run id rejection")
	}
}
