package main

import (
	"context"
	"testing"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/controlplane"
)

func TestRunArchiveCreatesVerifiableArchive(t *testing.T) {
	base := t.TempDir()
	eventsDir := base + "/events"
	snapshotDir := base + "/snapshots"
	archiveDir := base + "/archives"

	eventStore, err := controlplane.NewEventStore(eventsDir)
	if err != nil { t.Fatal(err) }
	runID := "run-cli-archive"
	for _, item := range []struct {
		typ controlplane.EventType
		taskID string
		agentID string
	}{
		{typ: controlplane.EventRunStarted},
		{typ: controlplane.EventTaskStarted, taskID: "task-1", agentID: "architect"},
	} {
		value, err := controlplane.NewEvent(controlplane.NewEventRequest{
			RunID: runID,
			Type: item.typ,
			TaskID: item.taskID,
			AgentID: item.agentID,
			Data: map[string]bool{"ok": true},
		}, time.Now().UTC())
		if err != nil { t.Fatal(err) }
		if _, err := eventStore.Append(context.Background(), value); err != nil { t.Fatal(err) }
	}
	snapshotStore, err := controlplane.NewSnapshotStore(snapshotDir)
	if err != nil { t.Fatal(err) }
	snapshot, err := snapshotStore.Create(context.Background(), eventStore, runID, time.Now().UTC())
	if err != nil { t.Fatal(err) }

	if err := runRunArchive([]string{
		runID,
		"--events-dir", eventsDir,
		"--snapshot-dir", snapshotDir,
		"--archive-dir", archiveDir,
	}); err != nil {
		t.Fatal(err)
	}

	archiveStore, err := controlplane.NewEventArchiveStore(archiveDir)
	if err != nil { t.Fatal(err) }
	record, err := archiveStore.Latest(runID)
	if err != nil { t.Fatal(err) }
	if err := controlplane.VerifyEventArchiveSnapshot(record, snapshot); err != nil {
		t.Fatal(err)
	}
	if record.ThroughSequence != 2 || len(record.Entries) != 2 {
		t.Fatalf("record=%#v", record)
	}
}

func TestParseRunArchiveArgs(t *testing.T) {
	options, err := parseRunArchiveArgs([]string{
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
	if _, err := parseRunArchiveArgs(nil); err == nil {
		t.Fatal("expected missing run id rejection")
	}
}
