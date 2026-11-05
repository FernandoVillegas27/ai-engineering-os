package main

import (
	"context"
	"strings"
	"testing"

	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/memory"
)

type fakeRunMemoryProvider struct {
	status memory.Status
	recall memory.RecallResult
	saves  []memory.SaveRequest
}

func (f *fakeRunMemoryProvider) ID() string { return "fake" }

func (f *fakeRunMemoryProvider) Status(context.Context) (memory.Status, error) {
	return f.status, nil
}

func (f *fakeRunMemoryProvider) Recall(_ context.Context, request memory.RecallRequest) (memory.RecallResult, error) {
	result := f.recall
	result.Provider = f.ID()
	result.Project = request.Project
	result.Query = request.Query
	return result, nil
}

func (f *fakeRunMemoryProvider) Save(_ context.Context, request memory.SaveRequest) (memory.SaveResult, error) {
	f.saves = append(f.saves, request)
	return memory.SaveResult{Provider: f.ID(), Project: request.Project, Saved: true}, nil
}

func TestInjectRunMemoryAddsBoundedUntrustedEvidenceWithoutMutatingInput(t *testing.T) {
	provider := &fakeRunMemoryProvider{
		status: memory.Status{Provider: "fake", Available: true, Healthy: true},
		recall: memory.RecallResult{Context: "Use the existing event-driven control plane."},
	}
	units := []execution.WorkUnit{{
		ID: "architect", AgentID: "architect",
		CollaborationContext: "preloaded context",
	}}

	got, recall := injectRunMemory(
		context.Background(), provider, "project", "extend control plane", units,
	)
	if !recall.Used {
		t.Fatalf("recall=%#v", recall)
	}
	if !strings.Contains(got[0].CollaborationContext, "preloaded context") ||
		!strings.Contains(got[0].CollaborationContext, "durable project memory") {
		t.Fatalf("context=%q", got[0].CollaborationContext)
	}
	if units[0].CollaborationContext != "preloaded context" {
		t.Fatalf("input mutated=%#v", units[0])
	}
}

func TestInjectRunMemoryUnavailableDoesNotBlock(t *testing.T) {
	provider := &fakeRunMemoryProvider{
		status: memory.Status{Provider: "fake", Available: false, Healthy: false, Details: "offline"},
	}
	units := []execution.WorkUnit{{ID: "a", AgentID: "architect"}}
	got, recall := injectRunMemory(context.Background(), provider, "project", "task", units)
	if recall.Used || !strings.Contains(recall.Warning, "offline") {
		t.Fatalf("recall=%#v", recall)
	}
	if got[0].CollaborationContext != "" {
		t.Fatalf("context=%q", got[0].CollaborationContext)
	}
}

func TestRunMemoryProviderModes(t *testing.T) {
	if provider, err := runMemoryProvider("off"); err != nil || provider != nil {
		t.Fatalf("off provider=%#v err=%v", provider, err)
	}
	if provider, err := runMemoryProvider("auto"); err != nil || provider == nil {
		t.Fatalf("auto provider=%#v err=%v", provider, err)
	}
	if _, err := runMemoryProvider("unknown"); err == nil {
		t.Fatal("expected unsupported provider error")
	}
}

func TestParseRunExecuteArgsMemoryMode(t *testing.T) {
	options, err := parseRunExecuteArgs([]string{
		"--task", "research",
		"--runtime", "codex",
		"--memory", "off",
	})
	if err != nil {
		t.Fatal(err)
	}
	if options.memoryMode != "off" {
		t.Fatalf("options=%#v", options)
	}

	defaults, err := parseRunExecuteArgs([]string{
		"--task", "research",
		"--runtime", "codex",
	})
	if err != nil {
		t.Fatal(err)
	}
	if defaults.memoryMode != "auto" {
		t.Fatalf("default memory=%q", defaults.memoryMode)
	}
}
