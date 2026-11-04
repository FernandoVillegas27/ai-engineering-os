package main

import (
	"testing"

	"github.com/FernandoVillegas27/ai-engineering-os/core/modelpolicy"
	"github.com/FernandoVillegas27/ai-engineering-os/core/runstate"
	"github.com/FernandoVillegas27/ai-engineering-os/core/scheduler"
)

func TestParseRunExecuteArgsDryRun(t *testing.T) {
	options, err := parseRunExecuteArgs([]string{
		"./project",
		"--task", "review backend and frontend",
		"--dry-run",
		"--project", "demo",
		"--max-concurrency", "4",
		"--json",
	})
	if err != nil {
		t.Fatal(err)
	}
	if options.root != "./project" || options.task != "review backend and frontend" {
		t.Fatalf("options = %#v", options)
	}
	if !options.dryRun || options.runtime != "" || options.projectID != "demo" {
		t.Fatalf("options = %#v", options)
	}
	if options.maxConcurrency != 4 || !options.json {
		t.Fatalf("options = %#v", options)
	}
}

func TestParseRunExecuteArgsRuntime(t *testing.T) {
	options, err := parseRunExecuteArgs([]string{
		"--task", "research architecture",
		"--runtime", "codex",
		"--ai",
	})
	if err != nil {
		t.Fatal(err)
	}
	if options.runtime != "codex" || options.dryRun || !options.useAI {
		t.Fatalf("options = %#v", options)
	}
	if options.maxConcurrency != 2 {
		t.Fatalf("default concurrency = %d", options.maxConcurrency)
	}
}

func TestParseRunExecuteArgsRejectsUnsafeCombinations(t *testing.T) {
	cases := [][]string{
		{"--task", "x"},
		{"--task", "x", "--dry-run", "--runtime", "codex"},
		{"--task", "x", "--runtime", "cursor"},
		{"--task", "x", "--dry-run", "--max-concurrency", "0"},
		{"--task", "x", "--dry-run", "--max-concurrency", "17"},
	}
	for _, args := range cases {
		if _, err := parseRunExecuteArgs(args); err == nil {
			t.Fatalf("expected error for %#v", args)
		}
	}
}

func TestSyncRunWorkUnitsUsesSchedulerTerminalState(t *testing.T) {
	run := runstate.Run{
		WorkUnits: []runstate.WorkUnit{
			{ID: "a", Status: runstate.WorkUnitQueued},
			{ID: "b", Status: runstate.WorkUnitQueued},
			{ID: "c", Status: runstate.WorkUnitQueued},
		},
	}
	syncRunWorkUnits(&run, []scheduler.Task{
		{ID: "a", Status: scheduler.StatusCompleted},
		{ID: "b", Status: scheduler.StatusFailed},
		{ID: "c", Status: scheduler.StatusBlocked},
	})
	if run.WorkUnits[0].Status != runstate.WorkUnitCompleted ||
		run.WorkUnits[1].Status != runstate.WorkUnitFailed ||
		run.WorkUnits[2].Status != runstate.WorkUnitBlocked {
		t.Fatalf("work units = %#v", run.WorkUnits)
	}
}


func TestParseRunExecuteArgsMultiAgentWriteOptIn(t *testing.T) {
	options, err := parseRunExecuteArgs([]string{
		"./project",
		"--task", "implement backend and frontend",
		"--runtime", "cursor",
		"--multi-agent-write",
		"--model-policy", "auto",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !options.multiAgentWrite || options.runtime != "cursor" {
		t.Fatalf("options = %#v", options)
	}
	if options.modelPolicy != modelpolicy.PolicyAuto {
		t.Fatalf("model policy = %s", options.modelPolicy)
	}
}

func TestParseRunExecuteArgsMultiAgentWriteRequiresExplicitCompatibleMode(t *testing.T) {
	cases := [][]string{
		{"--task", "x", "--dry-run", "--multi-agent-write"},
		{"--task", "x", "--runtime", "unknown", "--multi-agent-write"},
		{"--task", "x", "--runtime", "cursor"},
		{"--task", "x", "--runtime", "auto"},
	}
	for _, args := range cases {
		if _, err := parseRunExecuteArgs(args); err == nil {
			t.Fatalf("expected error for %#v", args)
		}
	}
}
