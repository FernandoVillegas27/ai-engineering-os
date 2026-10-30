package codex

import (
	"context"
	"crypto/sha256"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/proposal"
	"github.com/FernandoVillegas27/ai-engineering-os/core/staging"
	"github.com/FernandoVillegas27/ai-engineering-os/integrations/commandmodel"
)

type stageFakeRunner struct {
	invocation commandmodel.Invocation
	writeExtra bool
}

func (f *stageFakeRunner) LookPath(file string) (string, error) {
	return file, nil
}

func (f *stageFakeRunner) Run(_ context.Context, invocation commandmodel.Invocation) (string, string, error) {
	f.invocation = invocation
	target := filepath.Join(invocation.Dir, "service.go")
	if err := os.WriteFile(target, []byte("package sample\nfunc Value() int { return 2 }\n"), 0o644); err != nil {
		return "", "", err
	}
	if f.writeExtra {
		if err := os.WriteFile(filepath.Join(invocation.Dir, "extra.go"), []byte("package sample\n"), 0o644); err != nil {
			return "", "", err
		}
	}
	return "implemented", "", nil
}

func TestStageRuntimeWritesOnlyIsolatedWorkspace(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "service.go")
	original := []byte("package sample\nfunc Value() int { return 1 }\n")
	if err := os.WriteFile(source, original, 0o600); err != nil {
		t.Fatal(err)
	}

	runner := &stageFakeRunner{}
	runtime := StageRuntime{Binary: "codex", Runner: runner}
	p := proposal.Proposal{
		Summary: "Actualizar Value.",
		Files: []proposal.FileChange{
			{Path: "service.go", Action: "modify", Rationale: "Implement requested behavior."},
		},
	}

	result, err := runtime.Stage(context.Background(), executionUnitFixture(root), p)
	if err != nil {
		t.Fatalf("Stage() error = %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(result.Workspace) })

	after, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(original) {
		t.Fatal("original project changed")
	}

	staged, err := os.ReadFile(filepath.Join(result.Workspace, "service.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(staged), "return 2") {
		t.Fatalf("staged file = %q", staged)
	}
	if len(result.Changes) != 1 || result.Changes[0].Path != "service.go" || result.Changes[0].Action != "modify" {
		t.Fatalf("changes = %#v", result.Changes)
	}
	if runner.invocation.Dir == root {
		t.Fatal("Codex stage ran in original project")
	}
	joined := strings.Join(runner.invocation.Args, " ")
	if !strings.Contains(joined, "--sandbox workspace-write") {
		t.Fatalf("args = %q", joined)
	}
	if !strings.Contains(runner.invocation.Stdin, "ONLY the explicitly declared files") {
		t.Fatalf("prompt = %q", runner.invocation.Stdin)
	}
}

func TestStageRuntimeRecordsPreImageBeforeTheAgentRuns(t *testing.T) {
	root := t.TempDir()
	original := []byte("package sample\nfunc Value() int { return 1 }\n")
	if err := os.WriteFile(filepath.Join(root, "service.go"), original, 0o600); err != nil {
		t.Fatal(err)
	}
	pristine := t.TempDir()
	if _, err := execution.PrepareSafeReadOnlyView(root, pristine); err != nil {
		t.Fatal(err)
	}
	want, err := staging.SnapshotWorkspace(pristine, execution.DefaultSafeViewPolicy())
	if err != nil {
		t.Fatal(err)
	}

	p := proposal.Proposal{Summary: "x", Files: []proposal.FileChange{{Path: "service.go", Action: "modify"}}}
	result, err := (StageRuntime{Binary: "codex", Runner: &stageFakeRunner{}}).Stage(context.Background(), executionUnitFixture(root), p)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(result.Workspace) })

	if result.PreImage.IsZero() || result.PreImage.Digest() != want.Digest() {
		t.Fatalf("pre-image digest %q does not match the pristine view %q", result.PreImage.Digest(), want.Digest())
	}
	if got := result.PreImage.Snapshot()["service.go"]; got != sha256.Sum256(original) {
		t.Fatal("pre-image must hold the content hash from before the agent wrote")
	}
}

func TestStageRuntimeRejectsUndeclaredChange(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "service.go"), []byte("package sample\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	runner := &stageFakeRunner{writeExtra: true}
	runtime := StageRuntime{Binary: "codex", Runner: runner}
	p := proposal.Proposal{
		Summary: "Actualizar archivo.",
		Files: []proposal.FileChange{
			{Path: "service.go", Action: "modify", Rationale: "Requested."},
		},
	}

	if _, err := runtime.Stage(context.Background(), executionUnitFixture(root), p); err == nil {
		t.Fatal("expected undeclared change rejection")
	}
}

func TestStageRuntimeRejectsCreateOverExistingFile(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "service.go"), []byte("package sample\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	p := proposal.Proposal{
		Summary: "Crear archivo.",
		Files: []proposal.FileChange{
			{Path: "service.go", Action: "create", Rationale: "Requested."},
		},
	}
	if _, err := (StageRuntime{Runner: &stageFakeRunner{}}).Stage(context.Background(), executionUnitFixture(root), p); err == nil {
		t.Fatal("expected create precondition rejection")
	}
}
