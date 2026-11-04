package promotion

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/staging"
)

type ApplyRequest struct {
	Request
	Approved bool
}

type ApplyResult struct {
	Applied    bool
	Changes    []staging.WorkspaceChange
	RolledBack bool
}

func Apply(ctx context.Context, request ApplyRequest) (ApplyResult, error) {
	if !request.Approved {
		return ApplyResult{}, &Error{
			Kind:   KindApprovalRequired,
			Detail: "explicit user approval is required before promotion",
		}
	}

	plan, err := DryRun(ctx, request.Request)
	if err != nil {
		return ApplyResult{}, err
	}

	root, _ := filepath.Abs(request.OriginalRoot)
	staged, _ := filepath.Abs(request.StagedWorkspace)
	policy := execution.DefaultSafeViewPolicy()
	preImage := request.PreImage.Snapshot()

	parent := strings.TrimSpace(request.TempParent)
	if parent == "" {
		parent = filepath.Dir(root)
	}
	parent, _ = filepath.Abs(parent)

	backupRoot, err := os.MkdirTemp(parent, ".ai-engineering-os-rollback-*")
	if err != nil {
		return ApplyResult{}, &Error{
			Kind:   KindApplyFailed,
			Detail: "could not create rollback storage",
		}
	}
	cleanupBackup := true
	defer func() {
		if cleanupBackup {
			_ = os.RemoveAll(backupRoot)
		}
	}()

	for _, change := range plan.Changes {
		if err := backupTarget(root, backupRoot, change, preImage); err != nil {
			return ApplyResult{}, applyError("prepare rollback copy", change.Path, err, root, staged, backupRoot)
		}
	}

	var applied []staging.WorkspaceChange
	fail := func(change staging.WorkspaceChange, operation string, cause error) (ApplyResult, error) {
		rollbackErr := rollbackApplied(root, backupRoot, applied)
		if rollbackErr != nil {
			cleanupBackup = false
			return ApplyResult{
					Applied:    false,
					Changes:    append([]staging.WorkspaceChange(nil), applied...),
					RolledBack: false,
				}, &Error{
					Kind: KindRollbackFailed,
					Detail: fmt.Sprintf(
						"%s for %s; rollback also failed: %s; rollback storage was retained",
						operation,
						change.Path,
						scrubPath(rollbackErr.Error(), root, staged, backupRoot),
					),
					Paths: []string{change.Path},
				}
		}
		return ApplyResult{
				Applied:    false,
				Changes:    append([]staging.WorkspaceChange(nil), applied...),
				RolledBack: len(applied) > 0,
			}, applyError(operation, change.Path, cause, root, staged, backupRoot)
	}

	for _, change := range plan.Changes {
		if err := ctx.Err(); err != nil {
			return fail(change, "promotion canceled", err)
		}
		if err := verifyTargetState(root, change, preImage); err != nil {
			return fail(change, "original target changed after dry-run validation", err)
		}

		switch change.Action {
		case "create":
			if err := ensureSafeParent(root, change.Path, true); err != nil {
				return fail(change, "prepare destination", err)
			}
			source := filepath.Join(staged, filepath.FromSlash(change.Path))
			target := filepath.Join(root, filepath.FromSlash(change.Path))
			if err := copyNewRegularFile(source, target); err != nil {
				return fail(change, "create promoted file", err)
			}
		case "modify":
			if err := ensureSafeParent(root, change.Path, false); err != nil {
				return fail(change, "validate destination", err)
			}
			source := filepath.Join(staged, filepath.FromSlash(change.Path))
			target := filepath.Join(root, filepath.FromSlash(change.Path))
			if err := replaceRegularFile(source, target, preImage[change.Path]); err != nil {
				return fail(change, "replace promoted file", err)
			}
		case "delete":
			target := filepath.Join(root, filepath.FromSlash(change.Path))
			if err := os.Remove(target); err != nil {
				return fail(change, "delete promoted file", err)
			}
		default:
			return fail(change, "unsupported promotion action", fmt.Errorf("action %q", change.Action))
		}
		applied = append(applied, change)
	}

	stagedSnapshot, err := staging.SnapshotWorkspace(staged, policy)
	if err != nil {
		change := staging.WorkspaceChange{Path: "<workspace>", Action: "verify"}
		return fail(change, "reinspect staged workspace after promotion", err)
	}
	promotedSnapshot, err := snapshotSafeProject(root, parent, policy)
	if err != nil {
		change := staging.WorkspaceChange{Path: "<workspace>", Action: "verify"}
		return fail(change, "verify promoted project", err)
	}
	if diff := stagedSnapshot.DiffPaths(promotedSnapshot); len(diff) > 0 {
		change := staging.WorkspaceChange{Path: "<workspace>", Action: "verify"}
		return fail(change, "promoted project does not match staged workspace", fmt.Errorf("mismatched paths: %s", strings.Join(diff, ", ")))
	}

	return ApplyResult{
		Applied: true,
		Changes: append([]staging.WorkspaceChange(nil), applied...),
	}, nil
}

func backupTarget(root, backupRoot string, change staging.WorkspaceChange, preImage staging.WorkspaceSnapshot) error {
	switch change.Action {
	case "create":
		if _, existed := preImage[change.Path]; existed {
			return fmt.Errorf("create target existed in pre-image")
		}
		target := filepath.Join(root, filepath.FromSlash(change.Path))
		if _, err := os.Lstat(target); err == nil {
			return fmt.Errorf("create target already exists")
		} else if !os.IsNotExist(err) {
			return err
		}
		return nil
	case "modify", "delete":
		expected, existed := preImage[change.Path]
		if !existed {
			return fmt.Errorf("target did not exist in pre-image")
		}
		source := filepath.Join(root, filepath.FromSlash(change.Path))
		target := filepath.Join(backupRoot, filepath.FromSlash(change.Path))
		return copyExpectedRegularFile(source, target, expected)
	default:
		return fmt.Errorf("unsupported action %q", change.Action)
	}
}

func verifyTargetState(root string, change staging.WorkspaceChange, preImage staging.WorkspaceSnapshot) error {
	target := filepath.Join(root, filepath.FromSlash(change.Path))
	switch change.Action {
	case "create":
		if _, existed := preImage[change.Path]; existed {
			return fmt.Errorf("create target existed in pre-image")
		}
		if _, err := os.Lstat(target); err == nil {
			return fmt.Errorf("create target now exists")
		} else if !os.IsNotExist(err) {
			return err
		}
		return nil
	case "modify", "delete":
		expected, existed := preImage[change.Path]
		if !existed {
			return fmt.Errorf("target did not exist in pre-image")
		}
		actual, err := hashPinnedRegularFile(target)
		if err != nil {
			return err
		}
		if actual != expected {
			return fmt.Errorf("target content changed")
		}
		return nil
	default:
		return fmt.Errorf("unsupported action %q", change.Action)
	}
}

func ensureSafeParent(root, relative string, create bool) error {
	parts := strings.Split(filepath.ToSlash(relative), "/")
	current := root
	for _, part := range parts[:len(parts)-1] {
		current = filepath.Join(current, filepath.FromSlash(part))
		info, err := os.Lstat(current)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
				return fmt.Errorf("destination parent is not a regular directory")
			}
			continue
		}
		if !os.IsNotExist(err) {
			return err
		}
		if !create {
			return fmt.Errorf("destination parent no longer exists")
		}
		if err := os.Mkdir(current, 0o755); err != nil && !os.IsExist(err) {
			return err
		}
	}
	return nil
}

func copyExpectedRegularFile(source, target string, expected [32]byte) error {
	input, opened, err := openPinnedRegularFile(source)
	if err != nil {
		return err
	}
	defer input.Close()

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
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
		return fmt.Errorf("source content changed while preparing rollback")
	}

	after, err := input.Stat()
	if err != nil || !os.SameFile(opened, after) {
		_ = os.Remove(target)
		if err != nil {
			return err
		}
		return fmt.Errorf("source identity changed while preparing rollback")
	}
	return nil
}

func copyNewRegularFile(source, target string) error {
	input, opened, err := openPinnedRegularFile(source)
	if err != nil {
		return err
	}
	defer input.Close()

	output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		_ = os.Remove(target)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(target)
		return closeErr
	}
	after, err := input.Stat()
	if err != nil || !os.SameFile(opened, after) {
		_ = os.Remove(target)
		if err != nil {
			return err
		}
		return fmt.Errorf("staged source identity changed during copy")
	}
	return nil
}

func replaceRegularFile(source, target string, expected [32]byte) error {
	parent := filepath.Dir(target)
	temp, err := os.CreateTemp(parent, ".ai-engineering-os-apply-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	removeTemp := true
	defer func() {
		if removeTemp {
			_ = os.Remove(tempName)
		}
	}()

	input, opened, err := openPinnedRegularFile(source)
	if err != nil {
		_ = temp.Close()
		return err
	}
	_, copyErr := io.Copy(temp, input)
	closeErr := temp.Close()
	after, statErr := input.Stat()
	_ = input.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if statErr != nil {
		return statErr
	}
	if !os.SameFile(opened, after) {
		return fmt.Errorf("staged source identity changed during copy")
	}
	if err := os.Chmod(tempName, 0o644); err != nil {
		return err
	}

	current, err := hashPinnedRegularFile(target)
	if err != nil {
		return err
	}
	if current != expected {
		return fmt.Errorf("target content changed immediately before replacement")
	}
	if err := os.Remove(target); err != nil {
		return err
	}
	if err := os.Rename(tempName, target); err != nil {
		return err
	}
	removeTemp = false
	return nil
}

func openPinnedRegularFile(path string) (*os.File, os.FileInfo, error) {
	before, err := os.Lstat(path)
	if err != nil {
		return nil, nil, err
	}
	if before.Mode()&os.ModeSymlink != 0 || !before.Mode().IsRegular() {
		return nil, nil, fmt.Errorf("path is not a regular file")
	}
	if !os.SameFile(before, before) {
		return nil, nil, fmt.Errorf("cannot resolve file identity")
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
		return nil, nil, fmt.Errorf("file identity changed while opening")
	}
	return file, opened, nil
}

func hashPinnedRegularFile(path string) ([32]byte, error) {
	var zero [32]byte
	file, opened, err := openPinnedRegularFile(path)
	if err != nil {
		return zero, err
	}
	hash := sha256.New()
	_, copyErr := io.Copy(hash, file)
	after, statErr := file.Stat()
	_ = file.Close()
	if copyErr != nil {
		return zero, copyErr
	}
	if statErr != nil {
		return zero, statErr
	}
	if !os.SameFile(opened, after) {
		return zero, fmt.Errorf("file identity changed while hashing")
	}
	var sum [32]byte
	copy(sum[:], hash.Sum(nil))
	return sum, nil
}

func rollbackApplied(root, backupRoot string, applied []staging.WorkspaceChange) error {
	for i := len(applied) - 1; i >= 0; i-- {
		change := applied[i]
		target := filepath.Join(root, filepath.FromSlash(change.Path))
		switch change.Action {
		case "create":
			if err := removeRegularIfPresent(target); err != nil {
				return fmt.Errorf("rollback create %s: %w", change.Path, err)
			}
		case "modify", "delete":
			backup := filepath.Join(backupRoot, filepath.FromSlash(change.Path))
			if err := restoreRegularFile(backup, target); err != nil {
				return fmt.Errorf("rollback %s %s: %w", change.Action, change.Path, err)
			}
		default:
			return fmt.Errorf("rollback unsupported action %q", change.Action)
		}
	}
	return nil
}

func removeRegularIfPresent(path string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("rollback target is not a regular file")
	}
	return os.Remove(path)
}

func restoreRegularFile(backup, target string) error {
	if err := ensureSafeParent(filepath.Dir(filepath.Dir(target)), filepath.Base(filepath.Dir(target))+"/"+filepath.Base(target), true); err != nil {
		// The generic parent helper is intentionally not trusted for arbitrary
		// absolute paths here; fallback to a direct, symlink-aware directory check.
		parent := filepath.Dir(target)
		info, statErr := os.Lstat(parent)
		if statErr != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("rollback destination parent is unsafe")
		}
	}
	if err := removeRegularIfPresent(target); err != nil {
		return err
	}
	return copyNewRegularFile(backup, target)
}

func snapshotSafeProject(root, parent string, policy execution.SafeViewPolicy) (staging.WorkspaceSnapshot, error) {
	view, err := os.MkdirTemp(parent, ".ai-engineering-os-promotion-verify-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(view)
	if _, err := execution.PrepareSafeReadOnlyViewWithPolicy(root, view, policy); err != nil {
		return nil, err
	}
	return staging.SnapshotWorkspace(view, policy)
}

func applyError(operation, relative string, cause error, locations ...string) error {
	return &Error{
		Kind:   KindApplyFailed,
		Detail: operation + ": " + scrubPath(cause.Error(), locations...),
		Paths:  []string{relative},
	}
}
