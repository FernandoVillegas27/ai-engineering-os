package main

import (
	"context"
	"testing"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/controlplane"
)

func TestRunSnapshotCreatesVerifiableSnapshot(t *testing.T) {
	base := t.TempDir()
	eventsDir := base + "/events"
	snapshotDir := base + "/snapshots"

	eventStore, err := controlplane.NewEventStore(eventsDir)
	if err != nil {
		t.Fatal(err)
	}
	runID := "run-cli-snapshot"
	for _, item := range []struct {
		typ     controlplane.EventType
		taskID  string
		agentID string
	}{
		{typ: controlplane.EventRunStarted},
		{typ: controlplane.EventTaskStarted, taskID: "task-1", agentID: "architect"},
	} {
		value, err := controlplane.NewEvent(controlplane.NewEventRequest{
			RunID: runID, Type: item.typ, TaskID: item.taskID, AgentID: item.agentID,
			Data: map[string]bool{"ok": true},
		}, time.Now().UTC())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := eventStore.Append(context.Background(), value); err != nil {
			t.Fatal(err)
		}
	}

	if err := runRunSnapshot([]string{
		runID,
		"--events-dir", eventsDir,
		"--snapshot-dir", snapshotDir,
	}); err != nil {
		t.Fatal(err)
	}

	snapshotStore, err := controlplane.NewSnapshotStore(snapshotDir)
	if err != nil {
		t.Fatal(err)
	}
	record, err := snapshotStore.Latest(runID)
	if err != nil {
		t.Fatal(err)
	}
	if record.ThroughSequence != 2 || record.State.Status != controlplane.RunRunning {
		t.Fatalf("record=%#v", record)
	}
	if err := snapshotStore.Verify(context.Background(), eventStore, record); err != nil {
		t.Fatal(err)
	}
}

func TestParseRunSnapshotArgs(t *testing.T) {
	options, err := parseRunSnapshotArgs([]string{
		"run-1",
		"--events-dir", "./events",
		"--snapshot-dir", "./snapshots",
		"--json",
	})
	if err != nil {
		t.Fatal(err)
	}
	if options.runID != "run-1" ||
		options.eventsDir != "./events" ||
		options.snapshotDir != "./snapshots" ||
		!options.json {
		t.Fatalf("options=%#v", options)
	}
	if _, err := parseRunSnapshotArgs(nil); err == nil {
		t.Fatal("expected missing run id rejection")
	}
}
