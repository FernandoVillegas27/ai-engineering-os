package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/controlplane"\n\t"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/promotion"
	"github.com/FernandoVillegas27/ai-engineering-os/core/promotioncandidate"
	"github.com/FernandoVillegas27/ai-engineering-os/core/runstate"
	"github.com/FernandoVillegas27/ai-engineering-os/core/staging"
)

func TestParseRunPromoteArgsRequiresExplicitApprovalFlagAtExecution(t *testing.T) {
	options, err := parseRunPromoteArgs([]string{"run-123"})
	if err != nil {
		t.Fatal(err)
	}
	if options.approve {
		t.Fatalf("options = %#v", options)
	}
	if err := runRunPromote([]string{"run-123", "--promotion-dir", t.TempDir(), "--state-dir", t.TempDir(), "--events-dir", t.TempDir()}); err == nil {
		t.Fatal("expected explicit approval rejection")
	}
}

func TestRunPromoteAppliesPersistedCandidateAndAuditsApproval(t *testing.T) {
	base := t.TempDir()
	original := filepath.Join(base, "original")
	staged := filepath.Join(base, "staged")
	candidatesDir := filepath.Join(base, "candidates")
	stateDir := filepath.Join(base, "runs")
	eventsDir := filepath.Join(base, "events")
	for _, dir := range []string{original, staged, candidatesDir, stateDir, eventsDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(original, "main.go"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staged, "main.go"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	before, err := staging.SnapshotWorkspace(original, execution.DefaultSafeViewPolicy())
	if err != nil {
		t.Fatal(err)
	}
	request := promotion.Request{
		OriginalRoot:    original,
		StagedWorkspace: staged,
		PreImage:        staging.NewPreImage(before),
		Declared: []staging.WorkspaceChange{
			{Path: "main.go", Action: "modify"},
		},
		QualityPassed: true,
		TempParent:    base,
	}
	if plan, err := promotion.DryRun(context.Background(), request); err != nil || !plan.Ready {
		t.Fatalf("dry-run plan=%#v err=%v", plan, err)
	}

	now := time.Now().UTC()
	run, err := runstate.New(runstate.NewRunRequest{
		Task:        "update main",
		ProjectID:   "test-project",
		ProjectRoot: original,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := runstate.Transition(&run, runstate.StatusRunning, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := runstate.Transition(&run, runstate.StatusCompleted, now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	runStore, err := runstate.NewStore(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := runStore.Save(run); err != nil {
		t.Fatal(err)
	}

	record, err := promotioncandidate.New(run.ID, request, now)
	if err != nil {
		t.Fatal(err)
	}
	candidateStore, err := promotioncandidate.NewStore(candidatesDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := candidateStore.Save(record); err != nil {
		t.Fatal(err)
	}

	eventStore, err := controlplane.NewEventStore(eventsDir)
	if err != nil {
		t.Fatal(err)
	}
	started, err := controlplane.NewEvent(controlplane.NewEventRequest{
		RunID: run.ID,
		Type:  controlplane.EventRunStarted,
		Data:  map[string]bool{"test": true},
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := eventStore.Append(context.Background(), started); err != nil {
		t.Fatal(err)
	}
	requested, err := controlplane.NewEvent(controlplane.NewEventRequest{
		RunID: run.ID,
		Type:  controlplane.EventApprovalRequested,
		Data:  map[string]string{"kind": "promotion"},
	}, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := eventStore.Append(context.Background(), requested); err != nil {
		t.Fatal(err)
	}

	if err := runRunPromote([]string{
		run.ID,
		"--approve",
		"--promotion-dir", candidatesDir,
		"--state-dir", stateDir,
		"--events-dir", eventsDir,
	}); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(original, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new\n" {
		t.Fatalf("original content = %q", data)
	}

	loaded, err := runStore.Load(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Approvals) != 1 ||
		loaded.Approvals[0].Kind != "promotion" ||
		loaded.Approvals[0].Status != "granted" {
		t.Fatalf("approvals = %#v", loaded.Approvals)
	}

	events, err := eventStore.List(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 || events[2].Type != controlplane.EventApprovalGranted {
		t.Fatalf("events = %#v", events)
	}
}

func TestParseRunPromoteArgsRejectsInvalidInput(t *testing.T) {
	cases := [][]string{
		nil,
		{"run-1", "run-2"},
		{"run-1", "--promotion-dir"},
		{"run-1", "--state-dir"},
		{"run-1", "--events-dir"},
		{"run-1", "--unknown"},
	}
	for _, args := range cases {
		if _, err := parseRunPromoteArgs(args); err == nil {
			t.Fatalf("expected error for %#v", args)
		}
	}
}
