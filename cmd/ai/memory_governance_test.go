package main

import (
	"strings"
	"testing"
	"time"
)

func TestParseMemoryGovernanceCommands(t *testing.T) {
	list, err := parseMemoryGovernanceArgs("list", []string{"--project", "project-a", "--json"})
	if err != nil { t.Fatal(err) }
	if list.project != "project-a" || !list.json {
		t.Fatalf("list=%#v", list)
	}

	deleteOptions, err := parseMemoryGovernanceArgs("delete", []string{
		"memory-record-abc", "--project", "project-a", "--hard", "--yes",
	})
	if err != nil { t.Fatal(err) }
	if deleteOptions.governanceID != "memory-record-abc" || !deleteOptions.hard || !deleteOptions.yes {
		t.Fatalf("delete=%#v", deleteOptions)
	}

	supersede, err := parseMemoryGovernanceArgs("supersede", []string{
		"memory-record-abc", "--project", "project-a",
		"--title", "New decision", "--content", "Use v2", "--type", "decision",
	})
	if err != nil { t.Fatal(err) }
	if supersede.title != "New decision" || supersede.content != "Use v2" || supersede.memoryType != "decision" {
		t.Fatalf("supersede=%#v", supersede)
	}

	prune, err := parseMemoryGovernanceArgs("prune", []string{
		"--project", "project-a", "--older-than", "90d", "--yes",
	})
	if err != nil { t.Fatal(err) }
	if prune.olderThan != 90*24*time.Hour || !prune.yes {
		t.Fatalf("prune=%#v", prune)
	}
}

func TestMemoryGovernanceDestructiveConfirmation(t *testing.T) {
	if err := runMemoryDelete([]string{
		"memory-record-abc", "--project", "project-a", "--hard",
	}); err == nil || !strings.Contains(err.Error(), "requiere --yes") {
		t.Fatalf("hard delete err=%v", err)
	}

	if err := runMemoryPrune([]string{
		"--project", "project-a", "--older-than", "90d",
	}); err == nil || !strings.Contains(err.Error(), "requiere --yes") {
		t.Fatalf("prune err=%v", err)
	}
}

func TestParseRetentionDuration(t *testing.T) {
	cases := map[string]time.Duration{
		"1d": 24*time.Hour,
		"90d": 90*24*time.Hour,
		"2160h": 2160*time.Hour,
	}
	for input, want := range cases {
		got, err := parseRetentionDuration(input)
		if err != nil { t.Fatalf("%s: %v", input, err) }
		if got != want { t.Fatalf("%s=%s want=%s", input, got, want) }
	}
	for _, input := range []string{"0d", "-1d", "bad"} {
		if _, err := parseRetentionDuration(input); err == nil {
			t.Fatalf("expected invalid duration for %q", input)
		}
	}
}
