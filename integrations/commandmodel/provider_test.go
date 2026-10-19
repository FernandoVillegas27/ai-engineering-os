package commandmodel

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/FernandoVillegas27/ai-engineering-os/core/modelprovider"
)

type fakeAdapter struct{}

func (fakeAdapter) ID() string { return "fake-command" }

func (fakeAdapter) Build(_ modelprovider.ClassificationRequest, workDir string) (Invocation, error) {
	return Invocation{Command: "fake", Args: []string{"classify"}, Dir: workDir}, nil
}

func (fakeAdapter) Decode(workDir string) (modelprovider.ClassificationResponse, error) {
	data, err := os.ReadFile(filepath.Join(workDir, "result"))
	if err != nil {
		return modelprovider.ClassificationResponse{}, err
	}
	return modelprovider.ClassificationResponse{CanClassify: string(data) == "ok"}, nil
}

type fakeRunner struct {
	lookPathErr error
	runErr      error
	stderr      string
}

func (f fakeRunner) LookPath(file string) (string, error) {
	if f.lookPathErr != nil {
		return "", f.lookPathErr
	}
	return file, nil
}

func (f fakeRunner) Run(_ context.Context, invocation Invocation) (string, string, error) {
	if f.runErr != nil {
		return "", f.stderr, f.runErr
	}
	if err := os.WriteFile(filepath.Join(invocation.Dir, "result"), []byte("ok"), 0o600); err != nil {
		return "", "", err
	}
	return "", "", nil
}

func TestProviderRunsAdapterAndDecodes(t *testing.T) {
	provider := Provider{Adapter: fakeAdapter{}, Runner: fakeRunner{}}

	response, err := provider.Classify(context.Background(), modelprovider.ClassificationRequest{Task: "x"})
	if err != nil {
		t.Fatalf("Classify() error = %v", err)
	}
	if !response.CanClassify {
		t.Fatalf("response = %#v", response)
	}
}

func TestProviderReportsMissingCommand(t *testing.T) {
	provider := Provider{
		Adapter: fakeAdapter{},
		Runner:  fakeRunner{lookPathErr: errors.New("not found")},
	}

	_, err := provider.Classify(context.Background(), modelprovider.ClassificationRequest{Task: "x"})
	if err == nil {
		t.Fatal("expected missing command error")
	}
}

func TestProviderIncludesCommandStderr(t *testing.T) {
	provider := Provider{
		Adapter: fakeAdapter{},
		Runner:  fakeRunner{runErr: errors.New("exit 1"), stderr: "login required"},
	}

	_, err := provider.Classify(context.Background(), modelprovider.ClassificationRequest{Task: "x"})
	if err == nil {
		t.Fatal("expected execution error")
	}
}
