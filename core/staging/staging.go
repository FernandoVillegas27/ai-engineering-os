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

func parsePatchFiles(patchText string) ([]PatchFile, error) {
	lines := strings.Split(strings.ReplaceAll(patchText, "\r\n", "\n"), "\n")
	var files []PatchFile
	var pendingGitPath string
	seen := map[string]bool{}

	for i := 0; i < len(lines); i++ {
		line := lines[i]

		if strings.HasPrefix(line, "diff --git ") {
			if pendingGitPath != "" {
				return nil, fmt.Errorf("incomplete patch headers for %s", pendingGitPath)
			}
			fields := strings.Fields(line)
			if len(fields) != 4 || !strings.HasPrefix(fields[2], "a/") || !strings.HasPrefix(fields[3], "b/") {
				return nil, fmt.Errorf("unsupported diff header: %s", line)
			}
			oldPath := path.Clean(strings.TrimPrefix(fields[2], "a/"))
			newPath := path.Clean(strings.TrimPrefix(fields[3], "b/"))
			if oldPath != newPath {
				return nil, fmt.Errorf("renames are not supported in controlled staging: %s -> %s", oldPath, newPath)
			}
			pendingGitPath = newPath
			continue
		}

		if !strings.HasPrefix(line, "--- ") || i+1 >= len(lines) || !strings.HasPrefix(lines[i+1], "+++ ") {
			continue
		}

		oldPath, oldNull, err := parseUnifiedHeaderPath(strings.TrimPrefix(line, "--- "))
		if err != nil {
			return nil, err
		}
		newPath, newNull, err := parseUnifiedHeaderPath(strings.TrimPrefix(lines[i+1], "+++ "))
		if err != nil {
			return nil, err
		}

		var file PatchFile
		switch {
		case oldNull && newNull:
			return nil, fmt.Errorf("invalid patch: both sides are /dev/null")
		case oldNull:
			file.Path = newPath
			file.Action = "create"
		case newNull:
			file.Path = oldPath
			file.Action = "delete"
		default:
			if oldPath != newPath {
				return nil, fmt.Errorf("renames are not supported in controlled staging: %s -> %s", oldPath, newPath)
			}
			file.Path = newPath
			file.Action = "modify"
		}

		if pendingGitPath != "" {
			if file.Path != pendingGitPath {
				return nil, fmt.Errorf("diff header path %s does not match unified header path %s", pendingGitPath, file.Path)
			}
			pendingGitPath = ""
		}
		if seen[file.Path] {
			return nil, fmt.Errorf("duplicate patch file: %s", file.Path)
		}
		seen[file.Path] = true
		files = append(files, file)
		i++
	}

	if pendingGitPath != "" {
		return nil, fmt.Errorf("incomplete patch headers for %s", pendingGitPath)
	}
	return files, nil
}

func parseUnifiedHeaderPath(value string) (string, bool, error) {
	value = strings.TrimSpace(value)
	if value == "/dev/null" {
		return "", true, nil
	}
	if tab := strings.IndexByte(value, '\t'); tab >= 0 {
		value = value[:tab]
	}
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "a/") || strings.HasPrefix(value, "b/") {
		value = value[2:]
	}
	if value == "" {
		return "", false, fmt.Errorf("empty unified diff path")
	}
	if strings.ContainsAny(value, "\"'") {
		return "", false, fmt.Errorf("quoted or escaped unified diff paths are not supported: %s", value)
	}
	return path.Clean(value), false, nil
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
