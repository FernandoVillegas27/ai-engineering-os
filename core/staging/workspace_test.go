package staging

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/proposal"
)

func TestDetectWorkspaceChanges(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package a\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "delete.go"), []byte("package old\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	policy := execution.DefaultSafeViewPolicy()
	before, err := SnapshotWorkspace(root, policy)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "new.go"), []byte("package new\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "delete.go")); err != nil {
		t.Fatal(err)
	}

	changes, err := DetectWorkspaceChanges(before, root, policy)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 3 {
		t.Fatalf("changes = %#v", changes)
	}
	if changes[0].Path != "a.go" || changes[0].Action != "modify" {
		t.Fatalf("changes[0] = %#v", changes[0])
	}
	if changes[1].Path != "delete.go" || changes[1].Action != "delete" {
		t.Fatalf("changes[1] = %#v", changes[1])
	}
	if changes[2].Path != "new.go" || changes[2].Action != "create" {
		t.Fatalf("changes[2] = %#v", changes[2])
	}
}

func TestDetectWorkspaceChangesRejectsUnauthorizedFile(t *testing.T) {
	root := t.TempDir()
	policy := execution.DefaultSafeViewPolicy()
	before, err := SnapshotWorkspace(root, policy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("SECRET=x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := DetectWorkspaceChanges(before, root, policy); err == nil {
		t.Fatal("expected sensitive file rejection")
	}
}

func TestEnsureChangesMatchDeclared(t *testing.T) {
	declared, err := ValidateDeclaredFiles([]proposal.FileChange{
		{Path: "src/a.go", Action: "modify"},
		{Path: "src/new.go", Action: "create"},
	}, execution.DefaultSafeViewPolicy())
	if err != nil {
		t.Fatal(err)
	}
	actual := []WorkspaceChange{
		{Path: "src/a.go", Action: "modify"},
		{Path: "src/new.go", Action: "create"},
	}
	if err := EnsureChangesMatchDeclared(actual, declared); err != nil {
		t.Fatalf("EnsureChangesMatchDeclared() error = %v", err)
	}
}

func TestEnsureChangesMatchDeclaredRejectsUndeclared(t *testing.T) {
	declared := []WorkspaceChange{{Path: "src/a.go", Action: "modify"}}
	actual := []WorkspaceChange{
		{Path: "src/a.go", Action: "modify"},
		{Path: "src/b.go", Action: "create"},
	}
	if err := EnsureChangesMatchDeclared(actual, declared); err == nil {
		t.Fatal("expected undeclared change rejection")
	}
}
