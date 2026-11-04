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
	return OverlayTestWorkspaces(
		ctx,
		config,
		base,
		[]taskworkspace.Workspace{qa},
		map[string]taskworkspace.Inspection{qa.TaskID: inspection},
	)
}

func OverlayTestWorkspaces(
	ctx context.Context,
	config TestOverlayConfig,
	base IntegratedWorkspace,
	workspaces []taskworkspace.Workspace,
	inspections map[string]taskworkspace.Inspection,
) (IntegratedWorkspace, error) {
	if err := ctx.Err(); err != nil {
		return IntegratedWorkspace{}, fmt.Errorf("overlay tests: %w", err)
	}
	if strings.TrimSpace(base.Root) == "" || base.PreImage.IsZero() {
		return IntegratedWorkspace{}, errors.New("test overlay requires integrated base workspace")
	}
	if len(workspaces) == 0 {
		return IntegratedWorkspace{}, errors.New("test overlay requires at least one tests-only workspace")
	}
	if len(inspections) == 0 {
		return IntegratedWorkspace{}, errors.New("test overlay requires inspections")
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
	baseDigest := staging.NewPreImage(baseSnapshot).Digest()

	byTask := map[string]taskworkspace.Workspace{}
	var overlayChanges []staging.WorkspaceChange
	for _, qa := range workspaces {
		if strings.TrimSpace(qa.TaskID) == "" || strings.TrimSpace(qa.AgentID) == "" {
			return IntegratedWorkspace{}, errors.New("tests-only workspace requires task and agent ids")
		}
		if _, exists := byTask[qa.TaskID]; exists {
			return IntegratedWorkspace{}, fmt.Errorf("duplicate tests-only workspace: %s", qa.TaskID)
		}
		if strings.TrimSpace(qa.Root) == "" || qa.PreImage.IsZero() {
			return IntegratedWorkspace{}, fmt.Errorf("tests-only workspace %s has no root/pre-image", qa.TaskID)
		}
		if qa.PreImage.Digest() != baseDigest {
			return IntegratedWorkspace{}, fmt.Errorf(
				"tests-only workspace %s was not staged from the current integrated source result",
				qa.TaskID,
			)
		}
		inspection, ok := inspections[qa.TaskID]
		if !ok {
			return IntegratedWorkspace{}, fmt.Errorf("missing tests-only inspection for %s", qa.TaskID)
		}
		if inspection.ResultImage.IsZero() {
			return IntegratedWorkspace{}, fmt.Errorf("tests-only inspection %s has no frozen result image", qa.TaskID)
		}
		frozenChanges := staging.ChangesBetween(
			qa.PreImage.Snapshot(),
			inspection.ResultImage.Snapshot(),
		)
		if err := exactChanges(
			sortedWorkspaceChanges(frozenChanges),
			sortedWorkspaceChanges(inspection.Changes),
		); err != nil {
			return IntegratedWorkspace{}, fmt.Errorf(
				"tests-only inspection %s does not match frozen result image: %w",
				qa.TaskID,
				err,
			)
		}
		if err := staging.EnsureChangesMatchDeclared(inspection.Changes, qa.Declared); err != nil {
			return IntegratedWorkspace{}, fmt.Errorf("tests-only inspection %s violates declared scope: %w", qa.TaskID, err)
		}
		if err := filekind.ValidateTestsOnly(inspection.Changes); err != nil {
			return IntegratedWorkspace{}, fmt.Errorf("tests-only policy for %s: %w", qa.TaskID, err)
		}
		byTask[qa.TaskID] = qa
		overlayChanges = append(overlayChanges, inspection.Changes...)
	}

	for taskID := range inspections {
		if _, ok := byTask[taskID]; !ok {
			return IntegratedWorkspace{}, fmt.Errorf("inspection references unknown tests-only workspace %s", taskID)
		}
	}
	if err := rejectOverlayConflicts(base.Changes, overlayChanges); err != nil {
		return IntegratedWorkspace{}, err
	}
	if err := rejectDuplicateOverlayPaths(workspaces, inspections); err != nil {
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
	if staging.NewPreImage(copiedBase).Digest() != baseDigest {
		return IntegratedWorkspace{}, errors.New("test overlay copy does not match integrated base")
	}

	taskIDs := make([]string, 0, len(byTask))
	for taskID := range byTask {
		taskIDs = append(taskIDs, taskID)
	}
	sort.Strings(taskIDs)

	for _, taskID := range taskIDs {
		qa := byTask[taskID]
		inspection := inspections[taskID]
		resultSnapshot := inspection.ResultImage.Snapshot()
		for _, change := range sortedWorkspaceChanges(inspection.Changes) {
			var expected [32]byte
			hasExpected := false
			switch change.Action {
			case "create", "modify":
				var ok bool
				expected, ok = resultSnapshot[change.Path]
				if !ok {
					return IntegratedWorkspace{}, fmt.Errorf(
						"tests-only frozen result %s is missing %s",
						taskID,
						change.Path,
					)
				}
				hasExpected = true
			case "delete":
				if _, exists := resultSnapshot[change.Path]; exists {
					return IntegratedWorkspace{}, fmt.Errorf(
						"tests-only frozen result %s still contains deleted path %s",
						taskID,
						change.Path,
					)
				}
			}
			planned := PlannedChange{
				TaskID:  qa.TaskID,
				AgentID: qa.AgentID,
				Path:    change.Path,
				Action:  change.Action,
			}
			if err := applyPlannedChange(qa.Root, dir, planned, policy, expected, hasExpected); err != nil {
				return IntegratedWorkspace{}, fmt.Errorf(
					"apply tests-only change %s from %s: %w",
					change.Path,
					taskID,
					err,
				)
			}
		}
	}

	finalSnapshot, err := staging.SnapshotWorkspace(dir, policy)
	if err != nil {
		return IntegratedWorkspace{}, err
	}
	finalChanges := staging.ChangesBetween(base.PreImage.Snapshot(), finalSnapshot)
	expectedChanges := append([]staging.WorkspaceChange(nil), base.Changes...)
	expectedChanges = append(expectedChanges, overlayChanges...)
	if err := exactChanges(
		sortedWorkspaceChanges(finalChanges),
		sortedWorkspaceChanges(expectedChanges),
	); err != nil {
		return IntegratedWorkspace{}, fmt.Errorf("final source+tests workspace differs from expected overlay: %w", err)
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
			return fmt.Errorf("tests-only overlay conflicts with existing integrated change: %s", change.Path)
		}
	}
	return nil
}

func rejectDuplicateOverlayPaths(
	workspaces []taskworkspace.Workspace,
	inspections map[string]taskworkspace.Inspection,
) error {
	owners := map[string]string{}
	for _, workspace := range workspaces {
		inspection := inspections[workspace.TaskID]
		for _, change := range inspection.Changes {
			if owner, exists := owners[change.Path]; exists {
				return fmt.Errorf(
					"tests-only overlay conflict on %s between tasks %s and %s",
					change.Path,
					owner,
					workspace.TaskID,
				)
			}
			owners[change.Path] = workspace.TaskID
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
