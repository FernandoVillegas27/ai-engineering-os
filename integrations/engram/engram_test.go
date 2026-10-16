package engram

import (
	"context"
	"errors"
	"reflect"
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
		path:          "/usr/local/bin/engram",
		versionOutput: "engram v3.0.0",
		doctorOutput:  `{"status":"ok"}`,
	}

	client := Client{Binary: "engram", Runner: runner}
	status, err := client.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}

	if !status.Available || !status.Healthy {
		t.Fatalf("status = %#v", status)
	}
	if status.Version != "engram v3.0.0" {
		t.Fatalf("version = %q", status.Version)
	}
}

func TestStatusMissingBinary(t *testing.T) {
	runner := &fakeRunner{lookPathErr: errors.New("missing")}
	client := Client{Binary: "engram", Runner: runner}

	_, err := client.Status(context.Background())
	if err == nil {
		t.Fatal("expected lookup error")
	}
}

func TestMCPConfigUsesAgentProfile(t *testing.T) {
	client := New()
	got := client.MCP()

	wantArgs := []string{"mcp", "--tools=agent"}
	if got.Command != "engram" || !reflect.DeepEqual(got.Args, wantArgs) {
		t.Fatalf("MCP() = %#v", got)
	}
}

func TestSetupCommand(t *testing.T) {
	got, err := SetupCommand("cursor")
	if err != nil {
		t.Fatalf("SetupCommand() error = %v", err)
	}

	want := []string{"setup", "cursor"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SetupCommand() = %#v", got)
	}
}

func TestSetupCommandRejectsUnknownRuntime(t *testing.T) {
	if _, err := SetupCommand("unknown-runtime"); err == nil {
		t.Fatal("expected unsupported target error")
	}
}
