package runcheckpoint

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/integration"
	"github.com/FernandoVillegas27/ai-engineering-os/core/staging"
)

func writeCheckpointFile(t *testing.T, root, relative, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func checkpointFixture(t *testing.T) (string, integration.IntegratedWorkspace, string) {
	t.Helper()
	base := t.TempDir()
	original := filepath.Join(base, "project")
	integratedRoot := filepath.Join(base, "integrated")
	checkpointRoot := filepath.Join(base, "checkpoints")
	for _, dir := range []string{original, integratedRoot, checkpointRoot} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeCheckpointFile(t, original, "main.go", "package main\nconst value = 1\n")
	writeCheckpointFile(t, original, "pkg/keep.go", "package pkg\n")

	policy := execution.DefaultSafeViewPolicy()
	if _, err := execution.PrepareSafeReadOnlyViewWithPolicy(original, integratedRoot, policy); err != nil {
		t.Fatal(err)
	}
	before, err := staging.SnapshotWorkspace(original, policy)
	if err != nil {
		t.Fatal(err)
	}
	preImage := staging.NewPreImage(before)
	writeCheckpointFile(t, integratedRoot, "main.go", "package main\nconst value = 2\n")
	writeCheckpointFile(t, integratedRoot, "pkg/new.go", "package pkg\nconst created = true\n")
	changes, err := staging.DetectWorkspaceChanges(before, integratedRoot, policy)
	if err != nil {
		t.Fatal(err)
	}
	return original, integration.IntegratedWorkspace{
		Root:     integratedRoot,
		PreImage: preImage,
		Changes:  changes,
	}, checkpointRoot
}

func TestStoreSaveAndRestoreIntegratedWorkspace(t *testing.T) {
	original, workspace, checkpointRoot := checkpointFixture(t)
	store, err := NewStore(Config{Root: checkpointRoot})
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Save(
		context.Background(),
		"run-123",
		PhaseSourceIntegrated,
		original,
		workspace,
		time.Now().UTC(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if record.PreImageDigest != workspace.PreImage.Digest() || record.WorkspaceDigest == "" {
		t.Fatalf("record=%#v", record)
	}

	if err := os.RemoveAll(workspace.Root); err != nil {
		t.Fatal(err)
	}
	restored, err := store.Restore(
		context.Background(),
		"run-123",
		PhaseSourceIntegrated,
		filepath.Dir(original),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(restored.Root)

	if restored.PreImage.Digest() != workspace.PreImage.Digest() {
		t.Fatalf("pre-image=%s want=%s", restored.PreImage.Digest(), workspace.PreImage.Digest())
	}
	if len(restored.Changes) != len(workspace.Changes) {
		t.Fatalf("changes=%#v want=%#v", restored.Changes, workspace.Changes)
	}
	data, err := os.ReadFile(filepath.Join(restored.Root, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "package main\nconst value = 2\n" {
		t.Fatalf("restored main.go=%q", data)
	}
	if _, err := os.Stat(filepath.Join(restored.Root, "pkg", "new.go")); err != nil {
		t.Fatalf("restored created file: %v", err)
	}
}

func TestRestoreRejectsChangedOriginal(t *testing.T) {
	original, workspace, checkpointRoot := checkpointFixture(t)
	store, err := NewStore(Config{Root: checkpointRoot})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Save(
		context.Background(), "run-original-change", PhaseSourceIntegrated,
		original, workspace, time.Now().UTC(),
	); err != nil {
		t.Fatal(err)
	}
	writeCheckpointFile(t, original, "main.go", "package main\nconst value = 999\n")
	_, err = store.Restore(
		context.Background(), "run-original-change", PhaseSourceIntegrated,
		filepath.Dir(original),
	)
	if err == nil || !strings.Contains(err.Error(), "original project no longer matches checkpoint pre-image") {
		t.Fatalf("err=%#v", err)
	}
}

func TestRestoreRejectsTamperedCheckpointWorkspace(t *testing.T) {
	original, workspace, checkpointRoot := checkpointFixture(t)
	store, err := NewStore(Config{Root: checkpointRoot})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Save(
		context.Background(), "run-tamper", PhaseSourceIntegrated,
		original, workspace, time.Now().UTC(),
	); err != nil {
		t.Fatal(err)
	}
	checkpointFile := filepath.Join(
		checkpointRoot, "run-tamper", string(PhaseSourceIntegrated), "workspace", "main.go",
	)
	if err := os.WriteFile(checkpointFile, []byte("tampered\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = store.Restore(
		context.Background(), "run-tamper", PhaseSourceIntegrated,
		filepath.Dir(original),
	)
	if err == nil || !strings.Contains(err.Error(), "checkpoint workspace digest mismatch") {
		t.Fatalf("err=%#v", err)
	}
}

func TestSaveIsImmutablePerRunAndPhase(t *testing.T) {
	original, workspace, checkpointRoot := checkpointFixture(t)
	store, err := NewStore(Config{Root: checkpointRoot})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Save(
		context.Background(), "run-immutable", PhaseFinalIntegrated,
		original, workspace, time.Now().UTC(),
	); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Save(
		context.Background(), "run-immutable", PhaseFinalIntegrated,
		original, workspace, time.Now().UTC(),
	); err == nil {
		t.Fatal("expected immutable phase checkpoint rejection")
	}
}

func TestLoadRejectsSymlinkPhaseDirectory(t *testing.T) {
	_, _, checkpointRoot := checkpointFixture(t)
	store, err := NewStore(Config{Root: checkpointRoot})
	if err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(checkpointRoot, "run-link")
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(checkpointRoot, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(runDir, string(PhaseSourceIntegrated))
	if err := os.Symlink(target, link); err != nil {
		if os.IsPermission(err) {
			t.Skip("symlink creation not permitted")
		}
		t.Fatal(err)
	}
	if _, err := store.Load("run-link", PhaseSourceIntegrated); err == nil {
		t.Fatal("expected symlink checkpoint rejection")
	}
}
