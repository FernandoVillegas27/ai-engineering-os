package gentleai

import (
	"context"
	"errors"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

type fakeRunner struct {
	path          string
	lookPathErr   error
	versionOutput string
	versionErr    error
	doctorOutput  string
	doctorErr     error
	calls         [][]string
}

func (f *fakeRunner) LookPath(file string) (string, error) {
	if f.lookPathErr != nil {
		return "", f.lookPathErr
	}
	return f.path, nil
}

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) (string, error) {
	call := append([]string{name}, args...)
	f.calls = append(f.calls, call)

	if len(args) > 0 && args[0] == "version" {
		return f.versionOutput, f.versionErr
	}
	if len(args) > 0 && args[0] == "doctor" {
		return f.doctorOutput, f.doctorErr
	}
	return "", nil
}

func TestStatusHealthy(t *testing.T) {
	runner := &fakeRunner{
		path:          "/usr/local/bin/gentle-ai",
		versionOutput: "gentle-ai 4.2.0",
		doctorOutput:  "Summary: 8 passed, 0 failed, 0 warnings\nStatus:  healthy",
	}

	client := Client{Binary: "gentle-ai", Runner: runner}
	status, err := client.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}

	if !status.Available || !status.Healthy {
		t.Fatalf("status = %#v", status)
	}
	if status.Version != "gentle-ai 4.2.0" {
		t.Fatalf("version = %q", status.Version)
	}
}


func TestStatusUnhealthyEvenWhenDoctorExitsZero(t *testing.T) {
	runner := &fakeRunner{
		path:          "C:/Users/test/go/bin/gentle-ai.exe",
		versionOutput: "gentle-ai 4.2.0",
		doctorOutput:  "Summary: 8 passed, 5 failed, 0 warnings\nStatus:  unhealthy",
		doctorErr:     nil,
	}

	client := Client{Binary: "gentle-ai", Runner: runner}
	status, err := client.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if !status.Available {
		t.Fatal("expected Gentle-AI to be available")
	}
	if status.Healthy {
		t.Fatalf("expected unhealthy status, got %#v", status)
	}
}

func TestStatusMissingBinary(t *testing.T) {
	client := Client{
		Binary: "gentle-ai",
		Runner: &fakeRunner{lookPathErr: exec.ErrNotFound},
	}

	status, err := client.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status.Available || status.Healthy {
		t.Fatalf("status = %#v", status)
	}
}

func TestPreviewInstallCursor(t *testing.T) {
	client := New()
	plan, err := client.PreviewInstall("cursor")
	if err != nil {
		t.Fatalf("PreviewInstall() error = %v", err)
	}

	want := []string{"install", "--dry-run", "--agent", "cursor", "--preset", "minimal"}
	if plan.Mutates {
		t.Fatal("preview install must not mutate")
	}
	if plan.Command != "gentle-ai" || !reflect.DeepEqual(plan.Args, want) {
		t.Fatalf("plan = %#v", plan)
	}
}

func TestPreviewSyncCursor(t *testing.T) {
	client := New()
	plan, err := client.PreviewSync("cursor")
	if err != nil {
		t.Fatalf("PreviewSync() error = %v", err)
	}

	want := []string{"sync", "--dry-run", "--agent", "cursor"}
	if plan.Mutates {
		t.Fatal("preview sync must not mutate")
	}
	if !reflect.DeepEqual(plan.Args, want) {
		t.Fatalf("plan = %#v", plan)
	}
}

func TestRejectsUnknownTarget(t *testing.T) {
	_, err := New().PreviewInstall("unknown")
	if err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("expected unsupported target error, got %v", err)
	}
}

func TestWindowsInstallCommandPinned(t *testing.T) {
	got := InstallCommandWindows()
	want := []string{
		"go",
		"install",
		"github.com/gentleman-programming/gentle-ai/v4/cmd/gentle-ai@v4.2.0",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("InstallCommandWindows() = %#v", got)
	}
}

func TestMissingRunnerErrorIsPreserved(t *testing.T) {
	client := Client{
		Binary: "gentle-ai",
		Runner: &fakeRunner{lookPathErr: errors.New("permission denied")},
	}
	_, err := client.Status(context.Background())
	if err == nil {
		t.Fatal("expected lookup error")
	}
}
