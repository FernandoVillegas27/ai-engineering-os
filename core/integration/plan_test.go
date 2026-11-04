package integration

import (
	"crypto/sha256"
	"strings"
	"testing"

	"github.com/FernandoVillegas27/ai-engineering-os/core/staging"
	"github.com/FernandoVillegas27/ai-engineering-os/core/taskworkspace"
)

func preImage(seed byte) staging.PreImage {
	var hash [32]byte
	hash[0] = seed
	return staging.NewPreImage(staging.WorkspaceSnapshot{
		"main.go": hash,
	})
}

func workspace(taskID, agentID string, image staging.PreImage, declared ...staging.WorkspaceChange) taskworkspace.Workspace {
	return taskworkspace.Workspace{
		TaskID:   taskID,
		AgentID:  agentID,
		PreImage: image,
		Declared: declared,
		Root:     "/not-exposed-in-plan",
	}
}

func frozenInspection(image staging.PreImage, changes ...staging.WorkspaceChange) taskworkspace.Inspection {
	snapshot := image.Snapshot()
	for _, change := range changes {
		switch change.Action {
		case "create", "modify":
			snapshot[change.Path] = sha256.Sum256([]byte(change.Path + ":" + change.Action))
		case "delete":
			delete(snapshot, change.Path)
		}
	}
	return taskworkspace.Inspection{
		Changes:     append([]staging.WorkspaceChange(nil), changes...),
		ResultImage: staging.NewPreImage(snapshot),
	}
}

func TestBuildProducesDeterministicRelativeIntegrationPlan(t *testing.T) {
	image := preImage(1)
	workspaces := []taskworkspace.Workspace{
		workspace("frontend", "frontend-engineer", image,
			staging.WorkspaceChange{Path: "frontend/app.ts", Action: "create"},
		),
		workspace("backend", "backend-engineer", image,
			staging.WorkspaceChange{Path: "main.go", Action: "modify"},
			staging.WorkspaceChange{Path: "backend/new.go", Action: "create"},
		),
	}
	inspections := map[string]taskworkspace.Inspection{
		"frontend": frozenInspection(image,
			staging.WorkspaceChange{Path: "frontend/app.ts", Action: "create"},
		),
		"backend": frozenInspection(image,
			staging.WorkspaceChange{Path: "backend/new.go", Action: "create"},
			staging.WorkspaceChange{Path: "main.go", Action: "modify"},
		),
	}

	plan, err := Build(workspaces, inspections)
	if err != nil {
		t.Fatal(err)
	}
	if plan.PreImageDigest != image.Digest() {
		t.Fatalf("digest = %q", plan.PreImageDigest)
	}
	if len(plan.Changes) != 3 {
		t.Fatalf("changes = %#v", plan.Changes)
	}
	got := []string{plan.Changes[0].Path, plan.Changes[1].Path, plan.Changes[2].Path}
	want := []string{"backend/new.go", "frontend/app.ts", "main.go"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("paths = %#v", got)
		}
	}
	for _, change := range plan.Changes {
		if strings.Contains(change.Path, "/not-exposed") {
			t.Fatalf("absolute workspace path leaked: %#v", change)
		}
	}
	if _, ok := plan.expectedSourceHash("backend", "main.go"); !ok {
		t.Fatal("modify source hash was not frozen")
	}
	if _, ok := plan.expectedSourceHash("frontend", "frontend/app.ts"); !ok {
		t.Fatal("create source hash was not frozen")
	}
}

func TestBuildRejectsDifferentPreImages(t *testing.T) {
	a := preImage(1)
	b := preImage(2)
	_, err := Build([]taskworkspace.Workspace{
		workspace("a", "architect", a, staging.WorkspaceChange{Path: "a.go", Action: "create"}),
		workspace("b", "backend-engineer", b, staging.WorkspaceChange{Path: "b.go", Action: "create"}),
	}, map[string]taskworkspace.Inspection{
		"a": frozenInspection(a, staging.WorkspaceChange{Path: "a.go", Action: "create"}),
		"b": frozenInspection(b, staging.WorkspaceChange{Path: "b.go", Action: "create"}),
	})
	if err == nil || !strings.Contains(err.Error(), "does not share the integration pre-image") {
		t.Fatalf("err = %#v", err)
	}
}

func TestBuildRejectsDeclaredConflictBeforeIntegration(t *testing.T) {
	image := preImage(1)
	_, err := Build([]taskworkspace.Workspace{
		workspace("a", "backend-engineer", image, staging.WorkspaceChange{Path: "main.go", Action: "modify"}),
		workspace("b", "frontend-engineer", image, staging.WorkspaceChange{Path: "main.go", Action: "modify"}),
	}, map[string]taskworkspace.Inspection{
		"a": frozenInspection(image, staging.WorkspaceChange{Path: "main.go", Action: "modify"}),
		"b": frozenInspection(image),
	})
	if err == nil || !strings.Contains(err.Error(), "declared task workspace conflict") {
		t.Fatalf("err = %#v", err)
	}
}

func TestBuildRejectsInspectionOutsideDeclaredScope(t *testing.T) {
	image := preImage(1)
	_, err := Build([]taskworkspace.Workspace{
		workspace("backend", "backend-engineer", image,
			staging.WorkspaceChange{Path: "allowed.go", Action: "create"},
		),
	}, map[string]taskworkspace.Inspection{
		"backend": frozenInspection(image, staging.WorkspaceChange{Path: "other.go", Action: "create"}),
	})
	if err == nil || !strings.Contains(err.Error(), "violates declared scope") {
		t.Fatalf("err = %#v", err)
	}
}

func TestBuildRequiresFrozenResultImage(t *testing.T) {
	image := preImage(1)
	_, err := Build([]taskworkspace.Workspace{
		workspace("backend", "backend-engineer", image,
			staging.WorkspaceChange{Path: "new.go", Action: "create"},
		),
	}, map[string]taskworkspace.Inspection{
		"backend": {Changes: []staging.WorkspaceChange{{Path: "new.go", Action: "create"}}},
	})
	if err == nil || !strings.Contains(err.Error(), "no frozen result image") {
		t.Fatalf("err = %#v", err)
	}
}

func TestBuildRejectsInspectionThatDoesNotMatchFrozenImage(t *testing.T) {
	image := preImage(1)
	result := staging.NewPreImage(image.Snapshot())
	_, err := Build([]taskworkspace.Workspace{
		workspace("backend", "backend-engineer", image,
			staging.WorkspaceChange{Path: "new.go", Action: "create"},
		),
	}, map[string]taskworkspace.Inspection{
		"backend": {
			Changes:     []staging.WorkspaceChange{{Path: "new.go", Action: "create"}},
			ResultImage: result,
		},
	})
	if err == nil || !strings.Contains(err.Error(), "does not match its frozen result image") {
		t.Fatalf("err = %#v", err)
	}
}

func TestBuildRequiresExactlyOneInspectionPerKnownTask(t *testing.T) {
	image := preImage(1)
	ws := []taskworkspace.Workspace{
		workspace("backend", "backend-engineer", image,
			staging.WorkspaceChange{Path: "new.go", Action: "create"},
		),
	}
	if _, err := Build(ws, map[string]taskworkspace.Inspection{}); err == nil {
		t.Fatal("expected missing inspection rejection")
	}
	if _, err := Build(ws, map[string]taskworkspace.Inspection{
		"backend": frozenInspection(image, staging.WorkspaceChange{Path: "new.go", Action: "create"}),
		"ghost":   frozenInspection(image),
	}); err == nil {
		t.Fatal("expected unknown inspection rejection")
	}
}

func TestBuildRejectsNoActualChanges(t *testing.T) {
	image := preImage(1)
	_, err := Build([]taskworkspace.Workspace{
		workspace("backend", "backend-engineer", image,
			staging.WorkspaceChange{Path: "new.go", Action: "create"},
		),
	}, map[string]taskworkspace.Inspection{
		"backend": frozenInspection(image),
	})
	if err == nil || !strings.Contains(err.Error(), "no actual changes") {
		t.Fatalf("err = %#v", err)
	}
}

func TestBuildRejectsDuplicateTaskWorkspace(t *testing.T) {
	image := preImage(1)
	_, err := Build([]taskworkspace.Workspace{
		workspace("same", "backend-engineer", image,
			staging.WorkspaceChange{Path: "a.go", Action: "create"},
		),
		workspace("same", "frontend-engineer", image,
			staging.WorkspaceChange{Path: "b.go", Action: "create"},
		),
	}, map[string]taskworkspace.Inspection{
		"same": frozenInspection(image, staging.WorkspaceChange{Path: "a.go", Action: "create"}),
	})
	if err == nil || !strings.Contains(err.Error(), "duplicate task workspace") {
		t.Fatalf("err = %#v", err)
	}
}
