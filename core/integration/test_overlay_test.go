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

func prepareQAWorkspace(
	t *testing.T,
	base IntegratedWorkspace,
	parent string,
	declared []staging.WorkspaceChange,
) (*taskworkspace.Manager, taskworkspace.Workspace) {
	t.Helper()
	manager, err := taskworkspace.New(taskworkspace.Config{
		OriginalRoot: base.Root,
		Parent:       parent,
	})
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := manager.Prepare(context.Background(), "qa-tests", "qa-engineer", declared)
	if err != nil {
		t.Fatal(err)
	}
	return manager, workspace
}

func TestOverlayTestsBuildsFinalWorkspaceWithoutChangingSourceIntegration(t *testing.T) {
	fixture := newMaterializeFixture(t)
	defer fixture.cleanup(t)

	base, err := Materialize(context.Background(), MaterializeConfig{
		OriginalRoot: fixture.original,
		Parent:       fixture.integrationParent,
	}, fixture.plan, []taskworkspace.Workspace{fixture.backend, fixture.frontend})
	if err != nil {
		t.Fatal(err)
	}
	defer Cleanup(base, fixture.integrationParent)

	qaParent := filepath.Join(fixture.base, "qa")
	if err := os.MkdirAll(qaParent, 0o755); err != nil {
		t.Fatal(err)
	}
	manager, qa := prepareQAWorkspace(t, base, qaParent, []staging.WorkspaceChange{
		{Path: "backend/service_test.go", Action: "create"},
	})
	defer manager.Cleanup(qa)

	writeIntegrationFile(t, qa.Root, "backend/service_test.go", "package backend\n")
	inspection, err := manager.Inspect(context.Background(), qa)
	if err != nil {
		t.Fatal(err)
	}

	final, err := OverlayTests(context.Background(), TestOverlayConfig{
		Parent: fixture.integrationParent,
	}, base, qa, inspection)
	if err != nil {
		t.Fatal(err)
	}
	defer Cleanup(final, fixture.integrationParent)

	if final.PreImage.Digest() != base.PreImage.Digest() {
		t.Fatalf("final pre-image changed: %s != %s", final.PreImage.Digest(), base.PreImage.Digest())
	}
	if len(final.Changes) != len(base.Changes)+1 {
		t.Fatalf("final changes=%#v base=%#v", final.Changes, base.Changes)
	}
	testData, err := os.ReadFile(filepath.Join(final.Root, "backend", "service_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(testData) != "package backend\n" {
		t.Fatalf("test data=%q", testData)
	}

	// Source integration stays unchanged.
	if _, err := os.Stat(filepath.Join(base.Root, "backend", "service_test.go")); !os.IsNotExist(err) {
		t.Fatalf("QA test leaked into source integration, err=%v", err)
	}
	// Original stays unchanged too.
	if _, err := os.Stat(filepath.Join(fixture.original, "backend", "service_test.go")); !os.IsNotExist(err) {
		t.Fatalf("QA test leaked into original, err=%v", err)
	}
}

func TestOverlayTestsRejectsSourceFileChange(t *testing.T) {
	fixture := newMaterializeFixture(t)
	defer fixture.cleanup(t)
	base, err := Materialize(context.Background(), MaterializeConfig{
		OriginalRoot: fixture.original,
		Parent:       fixture.integrationParent,
	}, fixture.plan, []taskworkspace.Workspace{fixture.backend, fixture.frontend})
	if err != nil {
		t.Fatal(err)
	}
	defer Cleanup(base, fixture.integrationParent)

	qaParent := filepath.Join(fixture.base, "qa-source")
	if err := os.MkdirAll(qaParent, 0o755); err != nil {
		t.Fatal(err)
	}
	manager, qa := prepareQAWorkspace(t, base, qaParent, []staging.WorkspaceChange{
		{Path: "backend/qa_helper.go", Action: "create"},
	})
	defer manager.Cleanup(qa)
	writeIntegrationFile(t, qa.Root, "backend/qa_helper.go", "package backend\n")
	inspection, err := manager.Inspect(context.Background(), qa)
	if err != nil {
		t.Fatal(err)
	}

	_, err = OverlayTests(context.Background(), TestOverlayConfig{
		Parent: fixture.integrationParent,
	}, base, qa, inspection)
	if err == nil || !strings.Contains(err.Error(), "tests-only") {
		t.Fatalf("err=%#v", err)
	}
}

func TestOverlayTestsRejectsIntegratedBaseChangedAfterQAStarted(t *testing.T) {
	fixture := newMaterializeFixture(t)
	defer fixture.cleanup(t)
	base, err := Materialize(context.Background(), MaterializeConfig{
		OriginalRoot: fixture.original,
		Parent:       fixture.integrationParent,
	}, fixture.plan, []taskworkspace.Workspace{fixture.backend, fixture.frontend})
	if err != nil {
		t.Fatal(err)
	}
	defer Cleanup(base, fixture.integrationParent)

	qaParent := filepath.Join(fixture.base, "qa-base-change")
	if err := os.MkdirAll(qaParent, 0o755); err != nil {
		t.Fatal(err)
	}
	manager, qa := prepareQAWorkspace(t, base, qaParent, []staging.WorkspaceChange{
		{Path: "backend/service_test.go", Action: "create"},
	})
	defer manager.Cleanup(qa)
	writeIntegrationFile(t, qa.Root, "backend/service_test.go", "package backend\n")
	inspection, err := manager.Inspect(context.Background(), qa)
	if err != nil {
		t.Fatal(err)
	}

	writeIntegrationFile(t, base.Root, "main.go", "package main\nconst value = 777\n")
	_, err = OverlayTests(context.Background(), TestOverlayConfig{
		Parent: fixture.integrationParent,
	}, base, qa, inspection)
	if err == nil || (!strings.Contains(err.Error(), "integrated base changed") &&
		!strings.Contains(err.Error(), "was not staged from the current integrated source result")) {
		t.Fatalf("err=%#v", err)
	}
}

func TestOverlayTestsRejectsQAWorkspaceChangedAfterInspection(t *testing.T) {
	fixture := newMaterializeFixture(t)
	defer fixture.cleanup(t)
	base, err := Materialize(context.Background(), MaterializeConfig{
		OriginalRoot: fixture.original,
		Parent:       fixture.integrationParent,
	}, fixture.plan, []taskworkspace.Workspace{fixture.backend, fixture.frontend})
	if err != nil {
		t.Fatal(err)
	}
	defer Cleanup(base, fixture.integrationParent)

	qaParent := filepath.Join(fixture.base, "qa-toctou")
	if err := os.MkdirAll(qaParent, 0o755); err != nil {
		t.Fatal(err)
	}
	manager, qa := prepareQAWorkspace(t, base, qaParent, []staging.WorkspaceChange{
		{Path: "backend/service_test.go", Action: "create"},
	})
	defer manager.Cleanup(qa)
	writeIntegrationFile(t, qa.Root, "backend/service_test.go", "package backend\nconst first = true\n")
	inspection, err := manager.Inspect(context.Background(), qa)
	if err != nil {
		t.Fatal(err)
	}
	writeIntegrationFile(t, qa.Root, "backend/service_test.go", "package backend\nconst changed = true\n")

	_, err = OverlayTests(context.Background(), TestOverlayConfig{
		Parent: fixture.integrationParent,
	}, base, qa, inspection)
	if err == nil || !strings.Contains(err.Error(), "source changed after inspection") {
		t.Fatalf("err=%#v", err)
	}
}

func TestOverlayTestsRejectsConflictWithExistingIntegratedChange(t *testing.T) {
	base := IntegratedWorkspace{
		Root:     t.TempDir(),
		PreImage: staging.NewPreImage(staging.WorkspaceSnapshot{}),
		Changes:  []staging.WorkspaceChange{{Path: "pkg/service_test.go", Action: "create"}},
	}
	inspection := taskworkspace.Inspection{
		Changes:     []staging.WorkspaceChange{{Path: "pkg/service_test.go", Action: "create"}},
		ResultImage: staging.NewPreImage(staging.WorkspaceSnapshot{}),
	}
	if err := rejectOverlayConflicts(base.Changes, inspection.Changes); err == nil {
		t.Fatal("expected source/QA path conflict")
	}
}


func TestOverlayTestWorkspacesCombinesDisjointTestsOnlyAgents(t *testing.T) {
	fixture := newMaterializeFixture(t)
	defer fixture.cleanup(t)
	base, err := Materialize(context.Background(), MaterializeConfig{
		OriginalRoot: fixture.original,
		Parent:       fixture.integrationParent,
	}, fixture.plan, []taskworkspace.Workspace{fixture.backend, fixture.frontend})
	if err != nil {
		t.Fatal(err)
	}
	defer Cleanup(base, fixture.integrationParent)

	qaParent := filepath.Join(fixture.base, "qa-multi")
	if err := os.MkdirAll(qaParent, 0o755); err != nil {
		t.Fatal(err)
	}
	manager, err := taskworkspace.New(taskworkspace.Config{OriginalRoot: base.Root, Parent: qaParent})
	if err != nil {
		t.Fatal(err)
	}
	qaA, err := manager.Prepare(context.Background(), "qa-a", "qa-engineer", []staging.WorkspaceChange{
		{Path: "backend/service_test.go", Action: "create"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Cleanup(qaA)
	qaB, err := manager.Prepare(context.Background(), "qa-b", "test-auditor", []staging.WorkspaceChange{
		{Path: "frontend/app.spec.ts", Action: "create"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Cleanup(qaB)

	writeIntegrationFile(t, qaA.Root, "backend/service_test.go", "package backend\n")
	writeIntegrationFile(t, qaB.Root, "frontend/app.spec.ts", "export const ok = true;\n")
	inspectionA, err := manager.Inspect(context.Background(), qaA)
	if err != nil {
		t.Fatal(err)
	}
	inspectionB, err := manager.Inspect(context.Background(), qaB)
	if err != nil {
		t.Fatal(err)
	}

	final, err := OverlayTestWorkspaces(
		context.Background(),
		TestOverlayConfig{Parent: fixture.integrationParent},
		base,
		[]taskworkspace.Workspace{qaB, qaA},
		map[string]taskworkspace.Inspection{
			"qa-a": inspectionA,
			"qa-b": inspectionB,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer Cleanup(final, fixture.integrationParent)

	if _, err := os.Stat(filepath.Join(final.Root, "backend", "service_test.go")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(final.Root, "frontend", "app.spec.ts")); err != nil {
		t.Fatal(err)
	}
	if len(final.Changes) != len(base.Changes)+2 {
		t.Fatalf("final changes=%#v", final.Changes)
	}
}

func TestOverlayTestWorkspacesRejectsDuplicateTestOwnership(t *testing.T) {
	changes := []staging.WorkspaceChange{{Path: "pkg/service_test.go", Action: "create"}}
	workspaces := []taskworkspace.Workspace{
		{TaskID: "qa-a", AgentID: "qa-a", Declared: changes},
		{TaskID: "qa-b", AgentID: "qa-b", Declared: changes},
	}
	inspections := map[string]taskworkspace.Inspection{
		"qa-a": {Changes: changes},
		"qa-b": {Changes: changes},
	}
	if err := rejectDuplicateOverlayPaths(workspaces, inspections); err == nil {
		t.Fatal("expected duplicate tests-only path rejection")
	}
}
