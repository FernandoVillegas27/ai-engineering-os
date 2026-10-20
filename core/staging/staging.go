package staging

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/proposal"
)

type PatchFile struct {
	Path   string
	Action string
}

func ValidateProposal(p proposal.Proposal, policy execution.SafeViewPolicy) ([]PatchFile, error) {
	if strings.TrimSpace(p.Patch) == "" {
		return nil, fmt.Errorf("proposal patch is empty")
	}

	files, err := parsePatchFiles(p.Patch)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("proposal patch contains no file changes")
	}

	declared := map[string]string{}
	for _, file := range p.Files {
		path, err := validatePath(file.Path, policy)
		if err != nil {
			return nil, fmt.Errorf("declared file %q: %w", file.Path, err)
		}
		action := strings.ToLower(strings.TrimSpace(file.Action))
		if action != "create" && action != "modify" && action != "delete" {
			return nil, fmt.Errorf("declared file %q has unsupported action %q", file.Path, file.Action)
		}
		if _, exists := declared[path]; exists {
			return nil, fmt.Errorf("duplicate declared file: %s", path)
		}
		declared[path] = action
	}

	if len(declared) != len(files) {
		return nil, fmt.Errorf("proposal files and patch files do not match")
	}

	for _, file := range files {
		path, err := validatePath(file.Path, policy)
		if err != nil {
			return nil, fmt.Errorf("patch file %q: %w", file.Path, err)
		}
		expected, ok := declared[path]
		if !ok {
			return nil, fmt.Errorf("patch changes undeclared file: %s", path)
		}
		if expected != file.Action {
			return nil, fmt.Errorf("action mismatch for %s: proposal=%s patch=%s", path, expected, file.Action)
		}
	}

	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

func parsePatchFiles(patch string) ([]PatchFile, error) {
	lines := strings.Split(strings.ReplaceAll(patch, "\r\n", "\n"), "\n")
	var files []PatchFile
	var current *PatchFile
	var sawOld, sawNew bool
	var oldNull, newNull bool

	finish := func() error {
		if current == nil {
			return nil
		}
		if !sawOld || !sawNew {
			return fmt.Errorf("incomplete patch headers for %s", current.Path)
		}
		switch {
		case oldNull && newNull:
			return fmt.Errorf("invalid patch for %s: both sides are /dev/null", current.Path)
		case oldNull:
			current.Action = "create"
		case newNull:
			current.Action = "delete"
		default:
			current.Action = "modify"
		}
		files = append(files, *current)
		return nil
	}

	for _, line := range lines {
		if strings.HasPrefix(line, "diff --git ") {
			if err := finish(); err != nil {
				return nil, err
			}
			fields := strings.Fields(line)
			if len(fields) != 4 || !strings.HasPrefix(fields[2], "a/") || !strings.HasPrefix(fields[3], "b/") {
				return nil, fmt.Errorf("unsupported diff header: %s", line)
			}
			oldPath := strings.TrimPrefix(fields[2], "a/")
			newPath := strings.TrimPrefix(fields[3], "b/")
			if oldPath != newPath {
				return nil, fmt.Errorf("renames are not supported in controlled staging: %s -> %s", oldPath, newPath)
			}
			current = &PatchFile{Path: path.Clean(newPath)}
			sawOld, sawNew, oldNull, newNull = false, false, false, false
			continue
		}
		if current == nil {
			continue
		}
		if strings.HasPrefix(line, "--- ") && !sawOld {
			sawOld = true
			oldNull = strings.TrimSpace(strings.TrimPrefix(line, "--- ")) == "/dev/null"
		}
		if strings.HasPrefix(line, "+++ ") && !sawNew {
			sawNew = true
			newNull = strings.TrimSpace(strings.TrimPrefix(line, "+++ ")) == "/dev/null"
		}
	}
	if err := finish(); err != nil {
		return nil, err
	}
	return files, nil
}

func validatePath(value string, policy execution.SafeViewPolicy) (string, error) {
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
