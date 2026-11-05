package integration

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/staging"
	"github.com/FernandoVillegas27/ai-engineering-os/core/taskworkspace"
)

type MaterializeConfig struct {
	OriginalRoot string
	Parent       string
	Policy       execution.SafeViewPolicy
}

type IntegratedWorkspace struct {
	Root     string
	PreImage staging.PreImage
	Changes  []staging.WorkspaceChange
}

func Materialize(
	ctx context.Context,
	config MaterializeConfig,
	plan Plan,
	workspaces []taskworkspace.Workspace,
) (IntegratedWorkspace, error) {
	if err := ctx.Err(); err != nil {
		return IntegratedWorkspace{}, fmt.Errorf("materialize integration workspace: %w", err)
	}
	if strings.TrimSpace(plan.PreImageDigest) == "" || len(plan.Changes) == 0 {
		return IntegratedWorkspace{}, errors.New("integration plan is empty")
	}

	root, parent, policy, err := resolveMaterializeConfig(config)
	if err != nil {
		return IntegratedWorkspace{}, err
	}
	byTask, err := indexWorkspaces(plan, workspaces)
	if err != nil {
		return IntegratedWorkspace{}, err
	}

	dir, err := os.MkdirTemp(parent, ".ai-engineering-os-integration-*")
	if err != nil {
		return IntegratedWorkspace{}, fmt.Errorf("create integration workspace: %w", err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(dir)
		}
	}()

	view, err := execution.PrepareSafeReadOnlyViewWithPolicy(root, dir, policy)
	if err != nil {
		return IntegratedWorkspace{}, err
	}
	_ = view

	baseSnapshot, err := staging.SnapshotWorkspace(dir, policy)
	if err != nil {
		return IntegratedWorkspace{}, err
	}
	preImage := staging.NewPreImage(baseSnapshot)
	if preImage.Digest() != plan.PreImageDigest {
		return IntegratedWorkspace{}, errors.New("original project no longer matches the integration pre-image")
	}

	for _, change := range plan.Changes {
		if err := ctx.Err(); err != nil {
			return IntegratedWorkspace{}, fmt.Errorf("materialize integration workspace: %w", err)
		}
		sourceWorkspace := byTask[change.TaskID]
		if sourceWorkspace.PreImage.Digest() != plan.PreImageDigest {
			return IntegratedWorkspace{}, fmt.Errorf("task workspace %s pre-image changed", change.TaskID)
		}
		var expectedSource [32]byte
		hasExpectedSource := false
		if change.Action == "create" || change.Action == "modify" {
			expectedSource, hasExpectedSource = plan.expectedSourceHash(change.TaskID, change.Path)
			if !hasExpectedSource {
				return IntegratedWorkspace{}, fmt.Errorf("integration plan has no frozen source hash for %s from task %s", change.Path, change.TaskID)
			}
		}
		if err := applyPlannedChange(
			sourceWorkspace.Root,
			dir,
			change,
			policy,
			expectedSource,
			hasExpectedSource,
		); err != nil {
			return IntegratedWorkspace{}, fmt.Errorf("apply %s from task %s: %w", change.Path, change.TaskID, err)
		}
	}

	actual, err := staging.DetectWorkspaceChanges(baseSnapshot, dir, policy)
	if err != nil {
		return IntegratedWorkspace{}, err
	}
	expected := plannedWorkspaceChanges(plan)
	if err := exactChanges(actual, expected); err != nil {
		return IntegratedWorkspace{}, fmt.Errorf("integrated workspace differs from plan: %w", err)
	}

	cleanup = false
	return IntegratedWorkspace{
		Root:     dir,
		PreImage: preImage,
		Changes:  actual,
	}, nil
}

func Cleanup(workspace IntegratedWorkspace, parent string) error {
	if strings.TrimSpace(workspace.Root) == "" {
		return nil
	}
	root, err := filepath.Abs(workspace.Root)
	if err != nil {
		return err
	}
	parent, err = filepath.Abs(parent)
	if err != nil {
		return err
	}
	if !within(parent, root) || root == parent {
		return errors.New("refusing to clean path outside integration workspace parent")
	}
	base := filepath.Base(root)
	if !strings.HasPrefix(base, ".ai-engineering-os-integration-") {
		return errors.New("refusing to clean non-integration workspace")
	}
	return os.RemoveAll(root)
}

func resolveMaterializeConfig(config MaterializeConfig) (string, string, execution.SafeViewPolicy, error) {
	root, err := filepath.Abs(strings.TrimSpace(config.OriginalRoot))
	if err != nil {
		return "", "", execution.SafeViewPolicy{}, fmt.Errorf("resolve original root: %w", err)
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return "", "", execution.SafeViewPolicy{}, errors.New("original root must be an existing directory")
	}

	parent := strings.TrimSpace(config.Parent)
	if parent == "" {
		parent = filepath.Dir(root)
	}
	parent, err = filepath.Abs(parent)
	if err != nil {
		return "", "", execution.SafeViewPolicy{}, fmt.Errorf("resolve integration parent: %w", err)
	}
	parentInfo, err := os.Stat(parent)
	if err != nil || !parentInfo.IsDir() {
		return "", "", execution.SafeViewPolicy{}, errors.New("integration parent must be an existing directory")
	}
	if within(root, parent) {
		return "", "", execution.SafeViewPolicy{}, errors.New("integration parent must be outside original project")
	}

	policy := config.Policy
	if len(policy.AllowedExtensions) == 0 && len(policy.AllowedBasenames) == 0 {
		policy = execution.DefaultSafeViewPolicy()
	}
	return root, parent, policy, nil
}

func indexWorkspaces(plan Plan, workspaces []taskworkspace.Workspace) (map[string]taskworkspace.Workspace, error) {
	index := map[string]taskworkspace.Workspace{}
	for _, workspace := range workspaces {
		if strings.TrimSpace(workspace.TaskID) == "" {
			return nil, errors.New("task workspace id is required")
		}
		if _, exists := index[workspace.TaskID]; exists {
			return nil, fmt.Errorf("duplicate task workspace: %s", workspace.TaskID)
		}
		index[workspace.TaskID] = workspace
	}
	for _, change := range plan.Changes {
		workspace, ok := index[change.TaskID]
		if !ok {
			return nil, fmt.Errorf("integration plan references missing task workspace %s", change.TaskID)
		}
		if workspace.AgentID != change.AgentID {
			return nil, fmt.Errorf("agent mismatch for task workspace %s", change.TaskID)
		}
		if !declares(workspace.Declared, change.Path, change.Action) {
			return nil, fmt.Errorf("task workspace %s no longer declares %s %s", change.TaskID, change.Action, change.Path)
		}
	}
	return index, nil
}

func declares(values []staging.WorkspaceChange, path, action string) bool {
	for _, value := range values {
		if value.Path == path && value.Action == action {
			return true
		}
	}
	return false
}

func applyPlannedChange(
	sourceRoot, targetRoot string,
	change PlannedChange,
	policy execution.SafeViewPolicy,
	expectedSource [32]byte,
	hasExpectedSource bool,
) error {
	clean, err := staging.ValidateWorkspacePath(change.Path, policy)
	if err != nil {
		return err
	}
	if clean != change.Path {
		return errors.New("planned path is not canonical")
	}
	source := filepath.Join(sourceRoot, filepath.FromSlash(clean))
	target := filepath.Join(targetRoot, filepath.FromSlash(clean))

	switch change.Action {
	case "create":
		if _, err := os.Lstat(target); err == nil {
			return errors.New("create target already exists in integration base")
		} else if !os.IsNotExist(err) {
			return err
		}
		if err := ensureParent(targetRoot, clean); err != nil {
			return err
		}
		if !hasExpectedSource {
			return errors.New("create source hash is required")
		}
		return copyRegular(source, target, true, expectedSource)
	case "modify":
		if err := validateRegularTarget(target); err != nil {
			return err
		}
		if !hasExpectedSource {
			return errors.New("modify source hash is required")
		}
		return replaceRegular(source, target, expectedSource)
	case "delete":
		if _, err := os.Lstat(source); err == nil {
			return errors.New("task workspace source changed after inspection: deleted path reappeared")
		} else if !os.IsNotExist(err) {
			return err
		}
		if err := validateRegularTarget(target); err != nil {
			return err
		}
		return os.Remove(target)
	default:
		return fmt.Errorf("unsupported planned action %q", change.Action)
	}
}

func validateRegularTarget(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errors.New("integration target is not a regular file")
	}
	return nil
}

func ensureParent(root, relative string) error {
	parts := strings.Split(filepath.ToSlash(relative), "/")
	current := root
	for _, part := range parts[:len(parts)-1] {
		current = filepath.Join(current, filepath.FromSlash(part))
		info, err := os.Lstat(current)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
				return errors.New("integration parent is not a regular directory")
			}
			continue
		}
		if !os.IsNotExist(err) {
			return err
		}
		if err := os.Mkdir(current, 0o755); err != nil && !os.IsExist(err) {
			return err
		}
	}
	return nil
}

func copyRegular(source, target string, exclusive bool, expected [32]byte) error {
	input, opened, err := openPinnedRegular(source)
	if err != nil {
		return err
	}
	defer input.Close()

	flags := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	if exclusive {
		flags |= os.O_EXCL
	}
	output, err := os.OpenFile(target, flags, 0o644)
	if err != nil {
		return err
	}

	hash := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(output, hash), input)
	closeErr := output.Close()
	if copyErr != nil {
		_ = os.Remove(target)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(target)
		return closeErr
	}
	var actual [32]byte
	copy(actual[:], hash.Sum(nil))
	if actual != expected {
		_ = os.Remove(target)
		return errors.New("task workspace source changed after inspection")
	}
	after, err := input.Stat()
	if err != nil {
		_ = os.Remove(target)
		return err
	}
	if !os.SameFile(opened, after) {
		_ = os.Remove(target)
		return errors.New("task workspace source identity changed during copy")
	}
	return nil
}

func replaceRegular(source, target string, expected [32]byte) error {
	temp, err := os.CreateTemp(filepath.Dir(target), ".ai-engineering-os-integrate-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	_ = temp.Close()
	_ = os.Remove(tempPath)
	defer os.Remove(tempPath)

	if err := copyRegular(source, tempPath, true, expected); err != nil {
		return err
	}
	if err := os.Remove(target); err != nil {
		return err
	}
	return os.Rename(tempPath, target)
}

func openPinnedRegular(path string) (*os.File, os.FileInfo, error) {
	before, err := os.Lstat(path)
	if err != nil {
		return nil, nil, err
	}
	if before.Mode()&os.ModeSymlink != 0 || !before.Mode().IsRegular() {
		return nil, nil, errors.New("task workspace source is not a regular file")
	}
	if !os.SameFile(before, before) {
		return nil, nil, errors.New("cannot resolve task workspace source identity")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	opened, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, nil, err
	}
	if !opened.Mode().IsRegular() || !os.SameFile(before, opened) {
		_ = file.Close()
		return nil, nil, errors.New("task workspace source identity changed while opening")
	}
	return file, opened, nil
}

func plannedWorkspaceChanges(plan Plan) []staging.WorkspaceChange {
	out := make([]staging.WorkspaceChange, 0, len(plan.Changes))
	for _, change := range plan.Changes {
		out = append(out, staging.WorkspaceChange{Path: change.Path, Action: change.Action})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return out[i].Action < out[j].Action
	})
	return out
}

func exactChanges(actual, expected []staging.WorkspaceChange) error {
	if len(actual) != len(expected) {
		return fmt.Errorf("change count mismatch: actual=%d expected=%d", len(actual), len(expected))
	}
	for i := range expected {
		if actual[i] != expected[i] {
			return fmt.Errorf(
				"change mismatch at %d: actual=%s:%s expected=%s:%s",
				i,
				actual[i].Action,
				actual[i].Path,
				expected[i].Action,
				expected[i].Path,
			)
		}
	}
	return nil
}

func within(root, candidate string) bool {
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}
