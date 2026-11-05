package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveControlPlaneDirPrefersExplicitPath(t *testing.T) {
	t.Setenv("AI_ENGINEERING_OS_EVENTS_DIR", filepath.Join(t.TempDir(), "env-events"))
	explicit := filepath.Join(t.TempDir(), "explicit-events")

	got, err := resolveControlPlaneDir(explicit)
	if err != nil {
		t.Fatal(err)
	}
	if got != explicit {
		t.Fatalf("resolveControlPlaneDir() = %q, want %q", got, explicit)
	}
}

func TestResolveControlPlaneDirUsesEnvironment(t *testing.T) {
	want := filepath.Join(t.TempDir(), "events")
	t.Setenv("AI_ENGINEERING_OS_EVENTS_DIR", want)

	got, err := resolveControlPlaneDir("")
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("resolveControlPlaneDir() = %q, want %q", got, want)
	}
}

func TestParseControlPlaneArgs(t *testing.T) {
	options := runCommandOptions{}
	id, err := parseControlPlaneArgs([]string{
		"run-123",
		"--events-dir", filepath.Join("tmp", "events"),
		"--json",
	}, &options)
	if err != nil {
		t.Fatal(err)
	}
	if id != "run-123" || !options.json || options.eventsDir != filepath.Join("tmp", "events") {
		t.Fatalf("id=%q options=%#v", id, options)
	}
}

func TestParseControlPlaneArgsRejectsMissingRunAndMixedHelp(t *testing.T) {
	options := runCommandOptions{}
	if _, err := parseControlPlaneArgs([]string{"--json"}, &options); err == nil {
		t.Fatal("expected missing run id error")
	}

	options = runCommandOptions{}
	if _, err := parseControlPlaneArgs([]string{"run-123", "--help"}, &options); err == nil {
		t.Fatal("expected mixed help rejection")
	}
}

func TestControlPlaneDefaultDirectoryIsOutsideCurrentRepositoryWhenConfigRootIsAvailable(t *testing.T) {
	t.Setenv("AI_ENGINEERING_OS_EVENTS_DIR", "")
	got, err := resolveControlPlaneDir("")
	if err != nil {
		t.Skipf("user config directory unavailable: %v", err)
	}
	if !filepath.IsAbs(got) {
		t.Fatalf("default control-plane directory must be absolute: %q", got)
	}
	if filepath.Base(got) != "events" {
		t.Fatalf("default control-plane directory = %q", got)
	}
	if _, err := os.Stat(got); err == nil {
		// Resolution must not need to create the directory. Existing user config
		// state is acceptable; opening the EventStore owns creation.
		return
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat default directory: %v", err)
	}
}
