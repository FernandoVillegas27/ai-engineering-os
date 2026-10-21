package staging

import (
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/proposal"
)

type WorkspaceChange struct {
	Path   string
	Action string
}

type WorkspaceSnapshot map[string][32]byte

func ValidateDeclaredFiles(files []proposal.FileChange, policy execution.SafeViewPolicy) ([]WorkspaceChange, error) {
	if len(files) == 0 {
		return nil, fmt.Errorf("proposal declares no files")
	}

	seen := map[string]bool{}
	out := make([]WorkspaceChange, 0, len(files))
	for _, file := range files {
		clean, err := ValidateWorkspacePath(file.Path, policy)
		if err != nil {
			return nil, fmt.Errorf("declared file %q: %w", file.Path, err)
		}
		action := strings.ToLower(strings.TrimSpace(file.Action))
		if action != "create" && action != "modify" && action != "delete" {
			return nil, fmt.Errorf("declared file %q has unsupported action %q", file.Path, file.Action)
		}
		if seen[clean] {
			return nil, fmt.Errorf("duplicate declared file: %s", clean)
		}
		seen[clean] = true
		out = append(out, WorkspaceChange{Path: clean, Action: action})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

func ValidateWorkspacePath(value string, policy execution.SafeViewPolicy) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("path is empty")
	}
	if strings.Contains(value, "\\") {
		return "", fmt.Errorf("backslash paths are not supported")
	}
	if strings.HasPrefix(value, "/") || (len(value) >= 2 && value[1] == ':') {
		return "", fmt.Errorf("absolute paths are not allowed")
	}

	clean := path.Clean(value)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("path traversal is not allowed")
	}
	if execution.IsSensitivePathWithPolicy(clean, policy) {
		return "", fmt.Errorf("sensitive path is not allowed")
	}
	if !execution.IsAllowedRuntimeFile(clean, policy) {
		return "", fmt.Errorf("file type is not authorized by safe-view policy")
	}
	return clean, nil
}

func SnapshotWorkspace(root string, policy execution.SafeViewPolicy) (WorkspaceSnapshot, error) {
	snapshot := WorkspaceSnapshot{}
	err := walkWorkspace(root, policy, func(relative, absolute string) error {
		hash, err := hashFile(absolute)
		if err != nil {
			return err
		}
		snapshot[relative] = hash
		return nil
	})
	if err != nil {
		return nil, err
	}
	return snapshot, nil
}

func DetectWorkspaceChanges(before WorkspaceSnapshot, root string, policy execution.SafeViewPolicy) ([]WorkspaceChange, error) {
	after := WorkspaceSnapshot{}
	err := walkWorkspace(root, policy, func(relative, absolute string) error {
		hash, err := hashFile(absolute)
		if err != nil {
			return err
		}
		after[relative] = hash
		return nil
	})
	if err != nil {
		return nil, err
	}

	var changes []WorkspaceChange
	for relative, hash := range after {
		old, existed := before[relative]
		switch {
		case !existed:
			changes = append(changes, WorkspaceChange{Path: relative, Action: "create"})
		case old != hash:
			changes = append(changes, WorkspaceChange{Path: relative, Action: "modify"})
		}
	}
	for relative := range before {
		if _, exists := after[relative]; !exists {
			changes = append(changes, WorkspaceChange{Path: relative, Action: "delete"})
		}
	}

	sort.Slice(changes, func(i, j int) bool { return changes[i].Path < changes[j].Path })
	return changes, nil
}

func EnsureChangesMatchDeclared(actual, declared []WorkspaceChange) error {
	if len(actual) == 0 {
		return fmt.Errorf("stage runtime produced no file changes")
	}

	expected := map[string]string{}
	for _, change := range declared {
		expected[change.Path] = change.Action
	}
	for _, change := range actual {
		action, ok := expected[change.Path]
		if !ok {
			return fmt.Errorf("stage runtime changed undeclared file: %s", change.Path)
		}
		if action != change.Action {
			return fmt.Errorf("stage runtime action mismatch for %s: actual=%s declared=%s", change.Path, change.Action, action)
		}
	}
	return nil
}

func walkWorkspace(root string, policy execution.SafeViewPolicy, visit func(relative, absolute string) error) error {
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("resolve workspace root: %w", err)
	}

	err = filepath.WalkDir(absoluteRoot, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if current == absoluteRoot {
			return nil
		}

		relative, err := filepath.Rel(absoluteRoot, current)
		if err != nil {
			return err
		}
		slash := filepath.ToSlash(filepath.Clean(relative))

		if entry.IsDir() {
			if execution.IsSensitivePathWithPolicy(slash, policy) {
				return fmt.Errorf("stage runtime created sensitive directory: %s", slash)
			}
			return nil
		}

		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("stage runtime created unsupported filesystem entry: %s", slash)
		}
		if _, err := ValidateWorkspacePath(slash, policy); err != nil {
			return fmt.Errorf("stage runtime created unauthorized file %s: %w", slash, err)
		}
		return visit(slash, current)
	})
	if err != nil {
		return fmt.Errorf("inspect stage workspace: %w", err)
	}
	return nil
}

func hashFile(filename string) ([32]byte, error) {
	var zero [32]byte
	file, err := os.Open(filename)
	if err != nil {
		return zero, err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return zero, err
	}
	var sum [32]byte
	copy(sum[:], hash.Sum(nil))
	return sum, nil
}
