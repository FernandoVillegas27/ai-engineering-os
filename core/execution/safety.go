package execution

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type SafeViewReport struct {
	Root          string
	CopiedFiles   int
	ExcludedPaths []string
}

type SafeViewPolicy struct {
	AllowedExtensions map[string]bool
	AllowedBasenames  map[string]bool
	ExcludedBasenames map[string]bool
	ExcludedPrefixes  []string
}

func DefaultSafeViewPolicy() SafeViewPolicy {
	extensions := []string{
		".c", ".cc", ".cpp", ".cs", ".css", ".dart", ".go", ".graphql", ".gql",
		".h", ".hpp", ".html", ".java", ".js", ".jsx", ".kt", ".kts", ".less",
		".md", ".mjs", ".mts", ".php", ".proto", ".py", ".rb", ".rs", ".sass",
		".scala", ".scss", ".sh", ".sql", ".svelte", ".swift", ".toml", ".ts",
		".tsx", ".vue", ".xml", ".yaml", ".yml",
	}
	basenames := []string{
		".dockerignore", ".editorconfig", ".eslintignore", ".eslintrc", ".gitignore",
		".prettierignore", ".prettierrc", "Dockerfile", "Gemfile", "Makefile",
		"Procfile", "Rakefile", "go.mod", "go.sum", "package.json", "package-lock.json",
		"pnpm-lock.yaml", "pom.xml", "settings.gradle", "settings.gradle.kts",
		"build.gradle", "build.gradle.kts", "tsconfig.json", "vite.config.js",
		"vite.config.ts", "angular.json",
	}
	excluded := []string{
		".npmrc", ".pypirc", ".netrc", "netrc", "credentials", "credentials.json",
		"secrets", "secrets.json", "secrets.yaml", "secrets.yml", "gradle.properties",
		"id_rsa", "id_ed25519", "id_dsa", "id_ecdsa", "service-account.json",
	}

	policy := SafeViewPolicy{
		AllowedExtensions: map[string]bool{},
		AllowedBasenames:  map[string]bool{},
		ExcludedBasenames: map[string]bool{},
		ExcludedPrefixes:  []string{"credential.", "credentials.", "secret.", "secrets.", "service-account."},
	}
	for _, extension := range extensions {
		policy.AllowedExtensions[strings.ToLower(extension)] = true
	}
	for _, basename := range basenames {
		policy.AllowedBasenames[strings.ToLower(basename)] = true
	}
	for _, basename := range excluded {
		policy.ExcludedBasenames[strings.ToLower(basename)] = true
	}
	return policy
}

func PrepareSafeReadOnlyView(projectRoot string, destination string) (SafeViewReport, error) {
	return PrepareSafeReadOnlyViewWithPolicy(projectRoot, destination, DefaultSafeViewPolicy())
}

func PrepareSafeReadOnlyViewWithPolicy(projectRoot string, destination string, policy SafeViewPolicy) (SafeViewReport, error) {
	root, err := filepath.Abs(projectRoot)
	if err != nil {
		return SafeViewReport{}, fmt.Errorf("resolve project root: %w", err)
	}
	if destination == "" {
		return SafeViewReport{}, fmt.Errorf("safe view destination is required")
	}
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return SafeViewReport{}, fmt.Errorf("create safe view: %w", err)
	}

	report := SafeViewReport{Root: destination}
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}

		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.Clean(relative)

		if IsSensitivePathWithPolicy(relative, policy) || IsRuntimeControlPath(relative) {
			report.ExcludedPaths = append(report.ExcludedPaths, filepath.ToSlash(relative))
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			report.ExcludedPaths = append(report.ExcludedPaths, filepath.ToSlash(relative))
			return nil
		}

		if entry.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() || !IsAllowedRuntimeFile(relative, policy) {
			report.ExcludedPaths = append(report.ExcludedPaths, filepath.ToSlash(relative))
			return nil
		}

		pinned, err := pinFileIdentity(info)
		if err != nil {
			return fmt.Errorf("%s: %w", relative, err)
		}
		target := filepath.Join(destination, relative)
		if err := copyVerifiedRegularFile(path, target, pinned); err != nil {
			return err
		}
		report.CopiedFiles++
		return nil
	})
	if err != nil {
		return SafeViewReport{}, fmt.Errorf("prepare safe read-only view: %w", err)
	}
	sort.Strings(report.ExcludedPaths)
	return report, nil
}

func IsAllowedRuntimeFile(relativePath string, policy SafeViewPolicy) bool {
	base := strings.ToLower(filepath.Base(relativePath))
	if policy.AllowedBasenames[base] {
		return true
	}
	return policy.AllowedExtensions[strings.ToLower(filepath.Ext(base))]
}

// IsRuntimeControlPath identifies repository-owned files that provider runtimes
// may auto-discover as instructions, skills, or tool configuration. These paths
// are excluded from sanitized runtime views so repository text cannot bypass
// AI Engineering OS prompt, permission, skill-registry, or tool-binding policy.
func IsRuntimeControlPath(relativePath string) bool {
	clean := filepath.ToSlash(filepath.Clean(relativePath))
	lower := strings.ToLower(clean)
	parts := strings.Split(lower, "/")

	for _, part := range parts {
		switch part {
		case ".cursor", ".codex", ".claude", ".agents":
			return true
		}
	}

	base := parts[len(parts)-1]
	switch base {
	case "agents.md", "agents.override.md", "claude.md":
		return true
	}
	return false
}

func IsSensitivePath(relativePath string) bool {
	return IsSensitivePathWithPolicy(relativePath, DefaultSafeViewPolicy())
}

func IsSensitivePathWithPolicy(relativePath string, policy SafeViewPolicy) bool {
	clean := filepath.ToSlash(filepath.Clean(relativePath))
	lower := strings.ToLower(clean)
	parts := strings.Split(lower, "/")

	for _, part := range parts {
		switch part {
		case ".git", ".ssh", ".aws", ".azure", ".kube", ".gnupg":
			return true
		}
	}

	base := parts[len(parts)-1]
	if base == ".env" || strings.HasPrefix(base, ".env.") {
		return true
	}
	if policy.ExcludedBasenames[base] {
		return true
	}
	for _, prefix := range policy.ExcludedPrefixes {
		if strings.HasPrefix(base, strings.ToLower(prefix)) {
			return true
		}
	}

	switch strings.ToLower(filepath.Ext(base)) {
	case ".pem", ".key", ".p12", ".pfx", ".jks", ".keystore":
		return true
	}

	return false
}

// pinnedFileInfo is a FileInfo whose identity was captured eagerly.
// On Windows os.Lstat and DirEntry.Info resolve the file index lazily on the first
// os.SameFile call, so an unpinned value silently follows a later replacement.
type pinnedFileInfo struct{ fs.FileInfo }

func pinFileIdentity(info fs.FileInfo) (pinnedFileInfo, error) {
	if info == nil || !info.Mode().IsRegular() {
		return pinnedFileInfo{}, fmt.Errorf("cannot pin identity of a non-regular file")
	}
	// SameFile with itself forces the lazy identity lookup; false means it could not be resolved.
	if !os.SameFile(info, info) {
		return pinnedFileInfo{}, fmt.Errorf("cannot resolve file identity")
	}
	return pinnedFileInfo{info}, nil
}

func copyVerifiedRegularFile(source, target string, expected pinnedFileInfo) error {
	if expected.FileInfo == nil || !expected.Mode().IsRegular() {
		return fmt.Errorf("source is not an expected regular file: %s", source)
	}

	beforeOpen, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if beforeOpen.Mode()&os.ModeSymlink != 0 || !beforeOpen.Mode().IsRegular() {
		return fmt.Errorf("source changed to a non-regular file before copy: %s", source)
	}
	if !os.SameFile(expected.FileInfo, beforeOpen) {
		return fmt.Errorf("source identity changed before copy: %s", source)
	}

	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()

	opened, err := input.Stat()
	if err != nil {
		return err
	}
	if !opened.Mode().IsRegular() || !os.SameFile(beforeOpen, opened) {
		return fmt.Errorf("source identity changed while opening: %s", source)
	}
	// The handle identity is resolved eagerly, so this ties the bytes we read to the inspected file.
	if !os.SameFile(expected.FileInfo, opened) {
		return fmt.Errorf("source is not the file that was inspected: %s", source)
	}

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	output, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}

	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Chmod(target, 0o644)
}
