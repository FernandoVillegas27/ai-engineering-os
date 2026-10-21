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
