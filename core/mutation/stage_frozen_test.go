package mutation

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/integration"
	"github.com/FernandoVillegas27/ai-engineering-os/core/proposal"
	"github.com/FernandoVillegas27/ai-engineering-os/core/staging"
	"github.com/FernandoVillegas27/ai-engineering-os/core/taskworkspace"
)

type fakeStageRuntime struct {
	parent          string
	reportMismatch  bool
	undeclaredWrite bool
	err             error
}

func (f fakeStageRuntime) ID() string { return "fake-stage" }

func (f fakeStageRuntime) Stage(_ context.Context, unit execution.WorkUnit, p proposal.Proposal) (staging.StageResult, error) {
	if f.err != nil {
		return staging.StageResult{}, f.err
	}
	workspace, err := os.MkdirTemp(f.parent, ".ai-engineering-os-stage-*")
	if err != nil {
		return staging.StageResult{}, err
	}
	if _, err := execution.PrepareSafeReadOnlyView(unit.ProjectRoot, workspace); err != nil {
		_ = os.RemoveAll(workspace)
		return staging.StageResult{}, err
	}
	policy := execution.DefaultSafeViewPolicy()
	before, err := staging.SnapshotWorkspace(workspace, policy)
	if err != nil {
		_ = os.RemoveAll(workspace)
		return staging.StageResult{}, err
	}
	for _, file := range p.Files {
		target := filepath.Join(workspace, filepath.FromSlash(file.Path))
		switch file.Action {
		case "create", "modify":
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return staging.StageResult{}, err
			}
			if err := os.WriteFile(target, []byte("changed by "+unit.AgentID+"\n"), 0o644); err != nil {
				return staging.StageResult{}, err
			}
		case "delete":
			if err := os.Remove(target); err != nil {
				return staging.StageResult{}, err
			}
		}
	}
	if f.undeclaredWrite {
		if err := os.WriteFile(filepath.Join(workspace, "undeclared.go"), []byte("package main\n"), 0o644); err != nil {
			return staging.StageResult{}, err
		}
	}
	actual, err := staging.DetectWorkspaceChanges(before, workspace, policy)
	if err != nil {
		return staging.StageResult{}, err
	}
	reported := append([]staging.WorkspaceChange(nil), actual...)
	if f.reportMismatch {
		reported = nil
	}
	return staging.StageResult{
		Workspace: workspace,
		Changes:   reported,
		Summary:   "staged",
		PreImage:  staging.NewPreImage(before),
	}, nil
}

func stageFixture(t *testing.T) (string, string) {
	t.Helper()
	base := t.TempDir()
	original := filepath.Join(base, "project")
	stages := filepath.Join(base, "stages")
	if err := os.MkdirAll(original, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(stages, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(original, "main.go"), []byte("package main\nconst value = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(original, "frontend"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(original, "frontend", "app.ts"), []byte("export const value = 1;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return original, stages
}

func frozenFor(unit execution.WorkUnit, path, action string) FrozenProposal {
	p := proposal.Proposal{
		Summary: unit.Task,
		Files: []proposal.FileChange{{
			Path:      path,
			Action:    action,
			Rationale: unit.Task,
		}},
		Verification: []string{unit.Task},
	}
	return FrozenProposal{
		TaskID:   unit.ID,
		AgentID:  unit.AgentID,
		Proposal: p,
		Declared: []staging.WorkspaceChange{{
			Path: path, Action: action,
		}},
	}
}

func cleanupStagedSet(t *testing.T, set StagedSet) {
	t.Helper()
	for _, workspace := range set.Workspaces {
		if err := os.RemoveAll(workspace.Root); err != nil {
			t.Fatal(err)
		}
	}
}

func TestStageFrozenProposalsProducesInspectedSharedWorkspaces(t *testing.T) {
	original, stages := stageFixture(t)
	units := []execution.WorkUnit{
		proposalUnit("backend", "backend-engineer", "backend change"),
		proposalUnit("frontend", "frontend-engineer", "frontend change"),
	}
	for index := range units {
		units[index].ProjectRoot = original
	}
	frozen := []FrozenProposal{
		frozenFor(units[0], "main.go", "modify"),
		frozenFor(units[1], "frontend/app.ts", "modify"),
	}

	set, err := StageFrozenProposals(context.Background(), units, frozen, func(execution.WorkUnit) (staging.Runtime, error) {
		return fakeStageRuntime{parent: stages}, nil
	}, StageConfig{MaxConcurrency: 2})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanupStagedSet(t, set)

	if len(set.Workspaces) != 2 || len(set.Inspections) != 2 {
		t.Fatalf("set = %#v", set)
	}
	if set.Workspaces[0].TaskID != "backend" || set.Workspaces[1].TaskID != "frontend" {
		t.Fatalf("workspaces = %#v", set.Workspaces)
	}
	if set.Workspaces[0].PreImage.Digest() != set.Workspaces[1].PreImage.Digest() {
		t.Fatal("staged workspaces do not share pre-image")
	}
	for _, workspace := range set.Workspaces {
		inspection := set.Inspections[workspace.TaskID]
		if inspection.ResultImage.IsZero() || len(inspection.Changes) != 1 {
			t.Fatalf("inspection %s = %#v", workspace.TaskID, inspection)
		}
	}

	plan, err := integration.Build(set.Workspaces, set.Inspections)
	if err != nil {
		t.Fatalf("staged set should be integration-ready: %v", err)
	}
	if len(plan.Changes) != 2 {
		t.Fatalf("plan = %#v", plan)
	}
}

func TestStageFrozenProposalsRejectsRuntimeChangeReportMismatch(t *testing.T) {
	original, stages := stageFixture(t)
	unit := proposalUnit("backend", "backend-engineer", "backend change")
	unit.ProjectRoot = original
	set, err := StageFrozenProposals(
		context.Background(),
		[]execution.WorkUnit{unit},
		[]FrozenProposal{frozenFor(unit, "main.go", "modify")},
		func(execution.WorkUnit) (staging.Runtime, error) {
			return fakeStageRuntime{parent: stages, reportMismatch: true}, nil
		},
		StageConfig{},
	)
	defer cleanupStagedSet(t, set)
	if err == nil || !strings.Contains(err.Error(), "stage runtime report mismatch") {
		t.Fatalf("err = %#v", err)
	}
}

func TestStageFrozenProposalsRejectsUndeclaredFilesystemChange(t *testing.T) {
	original, stages := stageFixture(t)
	unit := proposalUnit("backend", "backend-engineer", "backend change")
	unit.ProjectRoot = original
	set, err := StageFrozenProposals(
		context.Background(),
		[]execution.WorkUnit{unit},
		[]FrozenProposal{frozenFor(unit, "main.go", "modify")},
		func(execution.WorkUnit) (staging.Runtime, error) {
			return fakeStageRuntime{parent: stages, undeclaredWrite: true}, nil
		},
		StageConfig{},
	)
	defer cleanupStagedSet(t, set)
	if err == nil || !strings.Contains(err.Error(), "undeclared") {
		t.Fatalf("err = %#v", err)
	}
}

func TestStageFrozenProposalsRejectsAgentMismatchBeforeRuntime(t *testing.T) {
	unit := proposalUnit("backend", "backend-engineer", "backend change")
	item := frozenFor(unit, "main.go", "modify")
	item.AgentID = "frontend-engineer"
	_, err := StageFrozenProposals(
		context.Background(),
		[]execution.WorkUnit{unit},
		[]FrozenProposal{item},
		func(execution.WorkUnit) (staging.Runtime, error) {
			t.Fatal("runtime resolver should not be called")
			return nil, nil
		},
		StageConfig{},
	)
	if err == nil || !strings.Contains(err.Error(), "agent mismatch") {
		t.Fatalf("err = %#v", err)
	}
}

func TestStageFrozenProposalsReturnsPartialSuccessfulWorkspacesOnFailure(t *testing.T) {
	original, stages := stageFixture(t)
	units := []execution.WorkUnit{
		proposalUnit("a", "backend-engineer", "backend change"),
		proposalUnit("b", "frontend-engineer", "frontend change"),
	}
	for i := range units {
		units[i].ProjectRoot = original
	}
	frozen := []FrozenProposal{
		frozenFor(units[0], "main.go", "modify"),
		frozenFor(units[1], "frontend/app.ts", "modify"),
	}
	sentinel := errors.New("stage unavailable")
	set, err := StageFrozenProposals(context.Background(), units, frozen, func(unit execution.WorkUnit) (staging.Runtime, error) {
		if unit.ID == "b" {
			return fakeStageRuntime{parent: stages, err: sentinel}, nil
		}
		return fakeStageRuntime{parent: stages}, nil
	}, StageConfig{MaxConcurrency: 1})
	defer cleanupStagedSet(t, set)
	if err == nil {
		t.Fatal("expected batch failure")
	}
}

func TestRequireSharedPreImageRejectsDifferentBases(t *testing.T) {
	var aHash [32]byte
	aHash[0] = 1
	var bHash [32]byte
	bHash[0] = 2
	err := requireSharedPreImage([]taskworkspace.Workspace{
		{TaskID: "a", PreImage: staging.NewPreImage(staging.WorkspaceSnapshot{"main.go": aHash})},
		{TaskID: "b", PreImage: staging.NewPreImage(staging.WorkspaceSnapshot{"main.go": bHash})},
	})
	if err == nil {
		t.Fatal("expected shared pre-image rejection")
	}
}
