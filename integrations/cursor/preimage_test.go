package cursor

import (
	"context"
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"

	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/proposal"
	"github.com/FernandoVillegas27/ai-engineering-os/core/staging"
)

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

	runner := &fakeRunner{writeFile: "service.go", stdout: "done"}
	p := proposal.Proposal{Summary: "x", Files: []proposal.FileChange{{Path: "service.go", Action: "modify"}}}
	result, err := (StageRuntime{Config: RunnerConfig{Runner: runner}}).Stage(context.Background(), unitFixture(root), p)
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
