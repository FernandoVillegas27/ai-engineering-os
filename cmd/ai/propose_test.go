package main

import "testing"

func TestParseProposeArgs(t *testing.T) {
	got, err := parseProposeArgs([]string{".", "--task", "Agregar validación", "--runtime", "codex"})
	if err != nil {
		t.Fatalf("parseProposeArgs() error = %v", err)
	}
	if got.root != "." || got.task != "Agregar validación" || got.runtime != "codex" {
		t.Fatalf("options = %#v", got)
	}
}

func TestParseProposeArgsDefaultsToAuto(t *testing.T) {
	got, err := parseProposeArgs([]string{"--task", "Revisar cambio"})
	if err != nil {
		t.Fatalf("parseProposeArgs() error = %v", err)
	}
	if got.runtime != "auto" {
		t.Fatalf("runtime = %q", got.runtime)
	}
}

func TestParseProposeArgsRejectsMissingTask(t *testing.T) {
	if _, err := parseProposeArgs([]string{"."}); err == nil {
		t.Fatal("expected missing task error")
	}
}

func TestParseProposeArgsRejectsUnsupportedRuntime(t *testing.T) {
	if _, err := parseProposeArgs([]string{"--task", "x", "--runtime", "claude"}); err == nil {
		t.Fatal("expected unsupported runtime error")
	}
}


func TestParseProposeArgsAcceptsCursorAndBalancedPolicy(t *testing.T) {
	got, err := parseProposeArgs([]string{
		"--task", "Implementar cambio",
		"--runtime", "cursor",
		"--model-policy", "balanced",
	})
	if err != nil {
		t.Fatalf("parseProposeArgs() error = %v", err)
	}
	if got.runtime != "cursor" || got.modelPolicy != "balanced" {
		t.Fatalf("options = %#v", got)
	}
}

func TestParseProposeArgsAcceptsExplicitModel(t *testing.T) {
	got, err := parseProposeArgs([]string{
		"--task", "Implementar cambio",
		"--runtime", "cursor",
		"--model", "gpt-5.6-sol-high",
	})
	if err != nil {
		t.Fatalf("parseProposeArgs() error = %v", err)
	}
	if got.model != "gpt-5.6-sol-high" {
		t.Fatalf("model = %q", got.model)
	}
}

func TestParseProposeArgsRejectsUnsupportedModelPolicy(t *testing.T) {
	if _, err := parseProposeArgs([]string{"--task", "x", "--model-policy", "turbo"}); err == nil {
		t.Fatal("expected unsupported model policy error")
	}
}
