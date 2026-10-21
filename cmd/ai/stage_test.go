package main

import "testing"

func TestParseStageArgs(t *testing.T) {
	got, err := parseStageArgs([]string{".", "--task", "Aplicar validación", "--runtime", "codex"})
	if err != nil {
		t.Fatalf("parseStageArgs() error = %v", err)
	}
	if got.root != "." || got.task != "Aplicar validación" || got.runtime != "codex" {
		t.Fatalf("options = %#v", got)
	}
}

func TestParseStageArgsDefaultsToAuto(t *testing.T) {
	got, err := parseStageArgs([]string{"--task", "Aplicar cambio"})
	if err != nil {
		t.Fatalf("parseStageArgs() error = %v", err)
	}
	if got.runtime != "auto" {
		t.Fatalf("runtime = %q", got.runtime)
	}
}

func TestParseStageArgsRejectsMissingTask(t *testing.T) {
	if _, err := parseStageArgs([]string{"."}); err == nil {
		t.Fatal("expected missing task error")
	}
}


func TestParseStageArgsAcceptsCursor(t *testing.T) {
	got, err := parseStageArgs([]string{"--task", "Aplicar cambio", "--runtime", "cursor"})
	if err != nil {
		t.Fatalf("parseStageArgs() error = %v", err)
	}
	if got.runtime != "cursor" {
		t.Fatalf("runtime = %q", got.runtime)
	}
}


func TestParseStageArgsDefaultsToAutoModelPolicy(t *testing.T) {
	got, err := parseStageArgs([]string{"--task", "Aplicar cambio"})
	if err != nil {
		t.Fatalf("parseStageArgs() error = %v", err)
	}
	if got.modelPolicy != "auto" {
		t.Fatalf("modelPolicy = %q", got.modelPolicy)
	}
}

func TestParseStageArgsAcceptsDeepPolicyAndModelOverride(t *testing.T) {
	got, err := parseStageArgs([]string{
		"--task", "Aplicar cambio",
		"--runtime", "cursor",
		"--model-policy", "deep",
		"--model", "custom-model",
	})
	if err != nil {
		t.Fatalf("parseStageArgs() error = %v", err)
	}
	if got.modelPolicy != "deep" || got.model != "custom-model" {
		t.Fatalf("options = %#v", got)
	}
}

func TestParseStageArgsRejectsUnsupportedModelPolicy(t *testing.T) {
	if _, err := parseStageArgs([]string{"--task", "x", "--model-policy", "turbo"}); err == nil {
		t.Fatal("expected unsupported model policy error")
	}
}
