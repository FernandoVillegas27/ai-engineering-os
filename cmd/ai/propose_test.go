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

func TestParseProposeArgsDefaultsToCodex(t *testing.T) {
	got, err := parseProposeArgs([]string{"--task", "Revisar cambio"})
	if err != nil {
		t.Fatalf("parseProposeArgs() error = %v", err)
	}
	if got.runtime != "codex" {
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
