package taskworkspace

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/FernandoVillegas27/ai-engineering-os/core/staging"
)

func writeFixtureFile(t *testing.T, root, relative, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func fixtureManager(t *testing.T) (*Manager, string, string) {
	t.Helper()
	base := t.TempDir()
	original := filepath.Join(base, "project")
	parent := filepath.Join(base, "workspaces")
	if err := os.MkdirAll(original, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFixtureFile(t, original, "main.go", "package main\nconst value = 1\n")
	writeFixtureFile(t, original, "backend/service.go", "package backend\n")
	writeFixtureFile(t, original, ".env", "SECRET=value")
	manager, err := New(Config{OriginalRoot: original, Parent: parent})
	if err != nil {
		t.Fatal(err)
	}
	return manager, original, parent
}

func TestPrepareCreatesIsolatedSafeWorkspaceWithPreImage(t *testing.T) {
	manager, original, _ := fixtureManager(t)
	workspace, err := manager.Prepare(context.Background(), "backend-task", "backend-engineer", []staging.WorkspaceChange{
		{Path: "main.go", Action: "modify"},
		{Path: "new.go", Action: "create"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Cleanup(workspace)

	if workspace.PreImage.IsZero() || workspace.PreImage.Len() != 2 {
		t.Fatalf("pre-image = %#v", workspace.PreImage)
	}
	if workspace.Copied != 2 {
		t.Fatalf("copied=%d", workspace.Copied)
	}
	if _, err := os.Stat(filepath.Join(workspace.Root, ".env")); !os.IsNotExist(err) {
		t.Fatalf("sensitive .env leaked into workspace, err=%v", err)
	}
	if workspace.Root == original {
		t.Fatal("workspace reused original project")
	}

	originalData, err := os.ReadFile(filepath.Join(original, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(originalData) != "package main\nconst value = 1\n" {
		t.Fatalf("original changed during prepare: %q", originalData)
	}
}

func TestInspectAcceptsOnlyDeclaredChanges(t *testing.T) {
	manager, original, _ := fixtureManager(t)
	workspace, err := manager.Prepare(context.Background(), "backend-task", "backend-engineer", []staging.WorkspaceChange{
		{Path: "main.go", Action: "modify"},
		{Path: "new.go", Action: "create"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Cleanup(workspace)

	writeFixtureFile(t, workspace.Root, "main.go", "package main\nconst value = 2\n")
	writeFixtureFile(t, workspace.Root, "new.go", "package main\n")

	inspection, err := manager.Inspect(context.Background(), workspace)
	if err != nil {
		t.Fatal(err)
	}
	if len(inspection.Changes) != 2 ||
		inspection.Changes[0].Path != "main.go" ||
		inspection.Changes[1].Path != "new.go" {
		t.Fatalf("changes = %#v", inspection.Changes)
	}

	data, err := os.ReadFile(filepath.Join(original, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "package main\nconst value = 1\n" {
		t.Fatalf("original mutated: %q", data)
	}
}

func TestInspectRejectsUndeclaredChange(t *testing.T) {
	manager, _, _ := fixtureManager(t)
	workspace, err := manager.Prepare(context.Background(), "backend-task", "backend-engineer", []staging.WorkspaceChange{
		{Path: "main.go", Action: "modify"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Cleanup(workspace)

	writeFixtureFile(t, workspace.Root, "backend/service.go", "package backend\nconst changed = true\n")
	if _, err := manager.Inspect(context.Background(), workspace); err == nil {
		t.Fatal("expected undeclared change rejection")
	}
}

func TestPrepareValidatesActionsAgainstPreImage(t *testing.T) {
	manager, _, _ := fixtureManager(t)
	if _, err := manager.Prepare(context.Background(), "bad-create", "backend-engineer", []staging.WorkspaceChange{
		{Path: "main.go", Action: "create"},
	}); err == nil {
		t.Fatal("expected create-existing rejection")
	}
	if _, err := manager.Prepare(context.Background(), "bad-modify", "backend-engineer", []staging.WorkspaceChange{
		{Path: "missing.go", Action: "modify"},
	}); err == nil {
		t.Fatal("expected modify-missing rejection")
	}
}

func TestValidateDisjointRejectsSameDeclaredFile(t *testing.T) {
	a := Workspace{
		TaskID:   "backend",
		Declared: []staging.WorkspaceChange{{Path: "shared.go", Action: "modify"}},
	}
	b := Workspace{
		TaskID:   "frontend",
		Declared: []staging.WorkspaceChange{{Path: "shared.go", Action: "modify"}},
	}
	if err := ValidateDisjoint([]Workspace{a, b}); err == nil {
		t.Fatal("expected declared file conflict")
	}

	b.Declared[0].Path = "frontend/app.ts"
	if err := ValidateDisjoint([]Workspace{a, b}); err != nil {
		t.Fatal(err)
	}
}

func TestCleanupRefusesOriginalProject(t *testing.T) {
	manager, original, _ := fixtureManager(t)
	if err := manager.Cleanup(Workspace{Root: original}); err == nil {
		t.Fatal("expected original cleanup rejection")
	}
	if _, err := os.Stat(original); err != nil {
		t.Fatalf("original project was removed: %v", err)
	}
}

func TestNewRejectsWorkspaceParentInsideOriginal(t *testing.T) {
	base := t.TempDir()
	original := filepath.Join(base, "project")
	parent := filepath.Join(original, "tmp")
	if err := os.MkdirAll(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := New(Config{OriginalRoot: original, Parent: parent}); err == nil {
		t.Fatal("expected inside-project parent rejection")
	}
}

func TestPrepareHonorsCancelledContext(t *testing.T) {
	manager, _, _ := fixtureManager(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := manager.Prepare(ctx, "task", "architect", []staging.WorkspaceChange{
		{Path: "main.go", Action: "modify"},
	}); err == nil {
		t.Fatal("expected canceled prepare")
	}
}
