package main

import (
	"testing"

	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
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


func TestMutationParticipantsContainOnlyExecutedPreStageAndSourceWriters(t *testing.T) {
	readOnly := []execution.WorkUnit{
		{ID: "research", AgentID: "researcher"},
		{ID: "architect", AgentID: "architect"},
	}
	mutating := []execution.WorkUnit{
		{ID: "backend", AgentID: "backend-engineer"},
		{ID: "frontend", AgentID: "frontend-engineer"},
	}
	participants := mutationParticipants(readOnly, mutating)
	if len(participants) != 4 {
		t.Fatalf("participants = %#v", participants)
	}
	want := []string{"architect", "backend", "frontend", "research"}
	for i, id := range want {
		if participants[i].ID != id {
			t.Fatalf("participants = %#v", participants)
		}
	}
}


func TestMutationParticipantTasksPreservesDependencies(t *testing.T) {
	full := []scheduler.Task{
		{ID: "research", AgentID: "researcher"},
		{ID: "architect", AgentID: "architect", DependsOn: []string{"research"}},
		{ID: "backend", AgentID: "backend-engineer", DependsOn: []string{"architect"}},
		{ID: "qa", AgentID: "qa-engineer", DependsOn: []string{"backend"}},
		{ID: "review", AgentID: "reviewer", DependsOn: []string{"qa"}},
	}
	participants := []execution.WorkUnit{
		{ID: "research", AgentID: "researcher"},
		{ID: "architect", AgentID: "architect"},
		{ID: "backend", AgentID: "backend-engineer", Mutation: execution.MutationCapability{SourceCode: true, Tests: true}},
		{ID: "qa", AgentID: "qa-engineer", Mutation: execution.MutationCapability{Tests: true}},
	}
	tasks, err := mutationParticipantTasks(full, participants)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 4 {
		t.Fatalf("tasks = %#v", tasks)
	}
	got := map[string][]string{}
	for _, task := range tasks {
		got[task.ID] = task.DependsOn
		if task.Status != scheduler.StatusQueued {
			t.Fatalf("task %s status=%s", task.ID, task.Status)
		}
	}
	if len(got["architect"]) != 1 || got["architect"][0] != "research" {
		t.Fatalf("architect deps=%#v", got["architect"])
	}
	if len(got["backend"]) != 1 || got["backend"][0] != "architect" {
		t.Fatalf("backend deps=%#v", got["backend"])
	}
	if len(got["qa"]) != 1 || got["qa"][0] != "backend" {
		t.Fatalf("qa deps=%#v", got["qa"])
	}
	for _, task := range tasks {
		switch task.ID {
		case "backend", "qa":
			if !task.Mutating {
				t.Fatalf("%s should be mutating: %#v", task.ID, task)
			}
		case "research", "architect":
			if task.Mutating {
				t.Fatalf("%s should remain read-only: %#v", task.ID, task)
			}
		}
	}
}

func TestMutationParticipantTasksRejectsExcludedDependency(t *testing.T) {
	full := []scheduler.Task{
		{ID: "architect", AgentID: "architect"},
		{ID: "backend", AgentID: "backend-engineer", DependsOn: []string{"architect"}},
	}
	_, err := mutationParticipantTasks(full, []execution.WorkUnit{
		{ID: "backend", AgentID: "backend-engineer"},
	})
	if err == nil {
		t.Fatal("expected excluded dependency rejection")
	}
}
