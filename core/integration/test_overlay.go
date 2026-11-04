package integration

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/filekind"
	"github.com/FernandoVillegas27/ai-engineering-os/core/staging"
	"github.com/FernandoVillegas27/ai-engineering-os/core/taskworkspace"
)

type TestOverlayConfig struct {
	Parent string
	Policy execution.SafeViewPolicy
}

func OverlayTests(
	ctx context.Context,
	config TestOverlayConfig,
	base IntegratedWorkspace,
	qa taskworkspace.Workspace,
	inspection taskworkspace.Inspection,
) (IntegratedWorkspace, error) {
	if err := ctx.Err(); err != nil {
		return IntegratedWorkspace{}, fmt.Errorf("overlay tests: %w", err)
	}
	if strings.TrimSpace(base.Root) == "" || base.PreImage.IsZero() {
		return IntegratedWorkspace{}, errors.New("test overlay requires integrated base workspace")
	}
	if strings.TrimSpace(qa.Root) == "" || qa.PreImage.IsZero() {
		return IntegratedWorkspace{}, errors.New("test overlay requires QA workspace and pre-image")
	}
	if inspection.ResultImage.IsZero() {
		return IntegratedWorkspace{}, errors.New("test overlay requires frozen QA result image")
	}

	policy := config.Policy
	if len(policy.AllowedExtensions) == 0 && len(policy.AllowedBasenames) == 0 {
		policy = execution.DefaultSafeViewPolicy()
	}
	parent := strings.TrimSpace(config.Parent)
	if parent == "" {
		parent = filepath.Dir(base.Root)
	}
	parent, err := filepath.Abs(parent)
	if err != nil {
		return IntegratedWorkspace{}, fmt.Errorf("resolve test overlay parent: %w", err)
	}
	info, err := os.Stat(parent)
	if err != nil || !info.IsDir() {
		return IntegratedWorkspace{}, errors.New("test overlay parent must be an existing directory")
	}

	baseSnapshot, err := staging.SnapshotWorkspace(base.Root, policy)
	if err != nil {
		return IntegratedWorkspace{}, fmt.Errorf("snapshot integrated base: %w", err)
	}
	baseActual := staging.ChangesBetween(base.PreImage.Snapshot(), baseSnapshot)
	if err := exactChanges(sortedWorkspaceChanges(baseActual), sortedWorkspaceChanges(base.Changes)); err != nil {
		return IntegratedWorkspace{}, fmt.Errorf("integrated base changed before QA overlay: %w", err)
	}
	if staging.NewPreImage(baseSnapshot).Digest() != qa.PreImage.Digest() {
		return IntegratedWorkspace{}, errors.New("QA workspace was not staged from the current integrated source result")
	}

	frozenQAChanges := staging.ChangesBetween(
		qa.PreImage.Snapshot(),
		inspection.ResultImage.Snapshot(),
	)
	if err := exactChanges(sortedWorkspaceChanges(frozenQAChanges), sortedWorkspaceChanges(inspection.Changes)); err != nil {
		return IntegratedWorkspace{}, fmt.Errorf("QA inspection does not match frozen result image: %w", err)
	}
	if err := staging.EnsureChangesMatchDeclared(inspection.Changes, qa.Declared); err != nil {
		return IntegratedWorkspace{}, fmt.Errorf("QA inspection violates declared scope: %w", err)
	}
	if err := filekind.ValidateTestsOnly(inspection.Changes); err != nil {
		return IntegratedWorkspace{}, fmt.Errorf("QA tests-only policy: %w", err)
	}
	if err := rejectOverlayConflicts(base.Changes, inspection.Changes); err != nil {
		return IntegratedWorkspace{}, err
	}

	dir, err := os.MkdirTemp(parent, ".ai-engineering-os-integration-tests-*")
	if err != nil {
		return IntegratedWorkspace{}, fmt.Errorf("create test overlay workspace: %w", err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(dir)
		}
	}()

	if _, err := execution.PrepareSafeReadOnlyViewWithPolicy(base.Root, dir, policy); err != nil {
		return IntegratedWorkspace{}, err
	}
	copiedBase, err := staging.SnapshotWorkspace(dir, policy)
	if err != nil {
		return IntegratedWorkspace{}, err
	}
	if staging.NewPreImage(copiedBase).Digest() != staging.NewPreImage(baseSnapshot).Digest() {
		return IntegratedWorkspace{}, errors.New("test overlay copy does not match integrated base")
	}

	resultSnapshot := inspection.ResultImage.Snapshot()
	for _, change := range sortedWorkspaceChanges(inspection.Changes) {
		var expected [32]byte
		hasExpected := false
		switch change.Action {
		case "create", "modify":
			var ok bool
			expected, ok = resultSnapshot[change.Path]
			if !ok {
				return IntegratedWorkspace{}, fmt.Errorf("QA frozen result is missing %s", change.Path)
			}
			hasExpected = true
		case "delete":
			if _, exists := resultSnapshot[change.Path]; exists {
				return IntegratedWorkspace{}, fmt.Errorf("QA frozen result still contains deleted path %s", change.Path)
			}
		}
		planned := PlannedChange{
			TaskID:  qa.TaskID,
			AgentID: qa.AgentID,
			Path:    change.Path,
			Action:  change.Action,
		}
		if err := applyPlannedChange(qa.Root, dir, planned, policy, expected, hasExpected); err != nil {
			return IntegratedWorkspace{}, fmt.Errorf("apply QA test change %s: %w", change.Path, err)
		}
	}

	finalSnapshot, err := staging.SnapshotWorkspace(dir, policy)
	if err != nil {
		return IntegratedWorkspace{}, err
	}
	finalChanges := staging.ChangesBetween(base.PreImage.Snapshot(), finalSnapshot)
	expectedChanges := append([]staging.WorkspaceChange(nil), base.Changes...)
	expectedChanges = append(expectedChanges, inspection.Changes...)
	if err := exactChanges(
		sortedWorkspaceChanges(finalChanges),
		sortedWorkspaceChanges(expectedChanges),
	); err != nil {
		return IntegratedWorkspace{}, fmt.Errorf("final source+QA workspace differs from expected overlay: %w", err)
	}

	cleanup = false
	return IntegratedWorkspace{
		Root:     dir,
		PreImage: base.PreImage,
		Changes:  sortedWorkspaceChanges(finalChanges),
	}, nil
}

func rejectOverlayConflicts(base, overlay []staging.WorkspaceChange) error {
	paths := map[string]bool{}
	for _, change := range base {
		paths[change.Path] = true
	}
	for _, change := range overlay {
		if paths[change.Path] {
			return fmt.Errorf("QA test overlay conflicts with existing integrated change: %s", change.Path)
		}
	}
	return nil
}

func sortedWorkspaceChanges(values []staging.WorkspaceChange) []staging.WorkspaceChange {
	out := append([]staging.WorkspaceChange(nil), values...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return out[i].Action < out[j].Action
	})
	return out
}
