package context7

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type fakeRunner struct {
	path        string
	lookPathErr error
	version     string
	versionErr  error
}

func (f *fakeRunner) LookPath(file string) (string, error) {
	if f.lookPathErr != nil {
		return "", f.lookPathErr
	}
	return f.path, nil
}

func (f *fakeRunner) Run(_ context.Context, _ string, args ...string) (string, error) {
	if len(args) == 1 && args[0] == "--version" {
		return f.version, f.versionErr
	}
	return "", nil
}

func TestStatusReadyWhenNPXExists(t *testing.T) {
	runner := &fakeRunner{
		path:    "C:/Program Files/nodejs/npx.cmd",
		version: "11.6.2",
	}
	client := Client{NPX: "npx", Runner: runner}

	status, err := client.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if !status.Available || !status.Ready {
		t.Fatalf("status = %#v", status)
	}
	if status.Version != "11.6.2" {
		t.Fatalf("version = %q", status.Version)
	}
}

func TestStatusMissingNPX(t *testing.T) {
	client := Client{
		NPX: "npx",
		Runner: &fakeRunner{
			lookPathErr: execNotFound(),
		},
	}

	status, err := client.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status.Available || status.Ready {
		t.Fatalf("status = %#v", status)
	}
}

func TestMCPConfigUsesPinnedPackage(t *testing.T) {
	got := New().MCP()
	wantArgs := []string{"-y", "@upstash/context7-mcp@4.4.2"}

	if got.Command != "npx" || !reflect.DeepEqual(got.Args, wantArgs) {
		t.Fatalf("MCP() = %#v", got)
	}
}

func TestRecommendedOperations(t *testing.T) {
	got := RecommendedOperations()
	want := []string{"resolve-library-id", "query-docs"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("RecommendedOperations() = %#v", got)
	}
}

func execNotFound() error {
	return errors.New("executable file not found in %PATH%")
}
