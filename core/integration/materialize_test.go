package integration

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FernandoVillegas27/ai-engineering-os/core/staging"
	"github.com/FernandoVillegas27/ai-engineering-os/core/taskworkspace"
)

func writeIntegrationFile(t *testing.T, root, relative, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

type materializeFixture struct {
	base              string
	original          string
	taskParent        string
	integrationParent string
	manager           *taskworkspace.Manager
	backend           taskworkspace.Workspace
	frontend          taskworkspace.Workspace
	inspections       map[string]taskworkspace.Inspection
	plan              Plan
}

func newMaterializeFixture(t *testing.T) materializeFixture {
	t.Helper()
	base := t.TempDir()
	original := filepath.Join(base, "project")
	taskParent := filepath.Join(base, "tasks")
	integrationParent := filepath.Join(base, "integrations")
	for _, dir := range []string{original, taskParent, integrationParent} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeIntegrationFile(t, original, "main.go", "package main\nconst value = 1\n")
	writeIntegrationFile(t, original, "frontend/app.ts", "export const value = 1;\n")
	writeIntegrationFile(t, original, "obsolete.go", "package main\nconst obsolete = true\n")
	writeIntegrationFile(t, original, ".env", "SECRET=value")

	manager, err := taskworkspace.New(taskworkspace.Config{
		OriginalRoot: original,
		Parent:       taskParent,
	})
	if err != nil {
		t.Fatal(err)
	}

	backend, err := manager.Prepare(context.Background(), "backend", "backend-engineer", []staging.WorkspaceChange{
		{Path: "main.go", Action: "modify"},
		{Path: "backend/new.go", Action: "create"},
		{Path: "obsolete.go", Action: "delete"},
	})
	if err != nil {
		t.Fatal(err)
	}
	frontend, err := manager.Prepare(context.Background(), "frontend", "frontend-engineer", []staging.WorkspaceChange{
		{Path: "frontend/app.ts", Action: "modify"},
	})
	if err != nil {
		t.Fatal(err)
	}

	writeIntegrationFile(t, backend.Root, "main.go", "package main\nconst value = 2\n")
	writeIntegrationFile(t, backend.Root, "backend/new.go", "package backend\nconst created = true\n")
	if err := os.Remove(filepath.Join(backend.Root, "obsolete.go")); err != nil {
		t.Fatal(err)
	}
	writeIntegrationFile(t, frontend.Root, "frontend/app.ts", "export const value = 2;\n")

	backendInspection, err := manager.Inspect(context.Background(), backend)
	if err != nil {
		t.Fatal(err)
	}
	frontendInspection, err := manager.Inspect(context.Background(), frontend)
	if err != nil {
		t.Fatal(err)
	}
	inspections := map[string]taskworkspace.Inspection{
		"backend":  backendInspection,
		"frontend": frontendInspection,
	}
	plan, err := Build([]taskworkspace.Workspace{backend, frontend}, inspections)
	if err != nil {
		t.Fatal(err)
	}

	return materializeFixture{
		base:              base,
		original:          original,
		taskParent:        taskParent,
		integrationParent: integrationParent,
		manager:           manager,
		backend:           backend,
		frontend:          frontend,
		inspections:       inspections,
		plan:              plan,
	}
}

func (f materializeFixture) cleanup(t *testing.T) {
	t.Helper()
	if err := f.manager.Cleanup(f.backend); err != nil {
		t.Fatal(err)
	}
	if err := f.manager.Cleanup(f.frontend); err != nil {
		t.Fatal(err)
	}
}

func TestMaterializeBuildsCombinedIsolatedWorkspace(t *testing.T) {
	f := newMaterializeFixture(t)
	defer f.cleanup(t)

	integrated, err := Materialize(context.Background(), MaterializeConfig{
		OriginalRoot: f.original,
		Parent:       f.integrationParent,
	}, f.plan, []taskworkspace.Workspace{f.backend, f.frontend})
	if err != nil {
		t.Fatal(err)
	}
	defer Cleanup(integrated, f.integrationParent)

	if integrated.PreImage.Digest() != f.plan.PreImageDigest {
		t.Fatalf("digest = %q", integrated.PreImage.Digest())
	}
	if len(integrated.Changes) != 4 {
		t.Fatalf("changes = %#v", integrated.Changes)
	}

	assertFile := func(relative, want string) {
		t.Helper()
		data, readErr := os.ReadFile(filepath.Join(integrated.Root, filepath.FromSlash(relative)))
		if readErr != nil {
			t.Fatal(readErr)
		}
		if string(data) != want {
			t.Fatalf("%s = %q want %q", relative, data, want)
		}
	}
	assertFile("main.go", "package main\nconst value = 2\n")
	assertFile("backend/new.go", "package backend\nconst created = true\n")
	assertFile("frontend/app.ts", "export const value = 2;\n")
	if _, err := os.Stat(filepath.Join(integrated.Root, "obsolete.go")); !os.IsNotExist(err) {
		t.Fatalf("obsolete.go should be deleted, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(integrated.Root, ".env")); !os.IsNotExist(err) {
		t.Fatalf("sensitive file leaked into integration workspace, err=%v", err)
	}

	originalMain, err := os.ReadFile(filepath.Join(f.original, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(originalMain) != "package main\nconst value = 1\n" {
		t.Fatalf("original changed: %q", originalMain)
	}
	if _, err := os.Stat(filepath.Join(f.original, "obsolete.go")); err != nil {
		t.Fatalf("original delete leaked: %v", err)
	}
}

func TestMaterializeRejectsOriginalChangedAfterTaskPreparation(t *testing.T) {
	f := newMaterializeFixture(t)
	defer f.cleanup(t)

	writeIntegrationFile(t, f.original, "main.go", "package main\nconst value = 999\n")
	_, err := Materialize(context.Background(), MaterializeConfig{
		OriginalRoot: f.original,
		Parent:       f.integrationParent,
	}, f.plan, []taskworkspace.Workspace{f.backend, f.frontend})
	if err == nil || !strings.Contains(err.Error(), "no longer matches the integration pre-image") {
		t.Fatalf("err = %#v", err)
	}
}

func TestMaterializeRejectsTaskWorkspaceChangedAfterInspection(t *testing.T) {
	f := newMaterializeFixture(t)
	defer f.cleanup(t)

	writeIntegrationFile(t, f.backend.Root, "main.go", "package main\nconst value = 777\n")
	_, err := Materialize(context.Background(), MaterializeConfig{
		OriginalRoot: f.original,
		Parent:       f.integrationParent,
	}, f.plan, []taskworkspace.Workspace{f.backend, f.frontend})
	if err == nil || !strings.Contains(err.Error(), "source changed after inspection") {
		t.Fatalf("err = %#v", err)
	}
}

func TestMaterializeRejectsMissingOrMismatchedTaskWorkspace(t *testing.T) {
	f := newMaterializeFixture(t)
	defer f.cleanup(t)

	_, err := Materialize(context.Background(), MaterializeConfig{
		OriginalRoot: f.original,
		Parent:       f.integrationParent,
	}, f.plan, []taskworkspace.Workspace{f.backend})
	if err == nil || !strings.Contains(err.Error(), "missing task workspace") {
		t.Fatalf("err = %#v", err)
	}

	badFrontend := f.frontend
	badFrontend.AgentID = "qa-engineer"
	_, err = Materialize(context.Background(), MaterializeConfig{
		OriginalRoot: f.original,
		Parent:       f.integrationParent,
	}, f.plan, []taskworkspace.Workspace{f.backend, badFrontend})
	if err == nil || !strings.Contains(err.Error(), "agent mismatch") {
		t.Fatalf("err = %#v", err)
	}
}

func TestCleanupRejectsNonIntegrationPath(t *testing.T) {
	base := t.TempDir()
	ordinary := filepath.Join(base, "ordinary")
	if err := os.Mkdir(ordinary, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Cleanup(IntegratedWorkspace{Root: ordinary}, base); err == nil {
		t.Fatal("expected cleanup boundary rejection")
	}
	if _, err := os.Stat(ordinary); err != nil {
		t.Fatalf("ordinary path was removed: %v", err)
	}
}


func TestMaterializeRejectsDeletedPathReappearingAfterInspection(t *testing.T) {
	f := newMaterializeFixture(t)
	defer f.cleanup(t)

	writeIntegrationFile(t, f.backend.Root, "obsolete.go", "package main\nconst resurrected = true\n")
	_, err := Materialize(context.Background(), MaterializeConfig{
		OriginalRoot: f.original,
		Parent:       f.integrationParent,
	}, f.plan, []taskworkspace.Workspace{f.backend, f.frontend})
	if err == nil || !strings.Contains(err.Error(), "deleted path reappeared") {
		t.Fatalf("err = %#v", err)
	}
}
