package execution

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type SafeViewReport struct {
	Root          string
	CopiedFiles   int
	ExcludedPaths []string
}

func PrepareSafeReadOnlyView(projectRoot string, destination string) (SafeViewReport, error) {
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

		if IsSensitivePath(relative) {
			report.ExcludedPaths = append(report.ExcludedPaths, filepath.ToSlash(relative))
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			report.ExcludedPaths = append(report.ExcludedPaths, filepath.ToSlash(relative))
			return nil
		}

		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !info.Mode().IsRegular() {
			report.ExcludedPaths = append(report.ExcludedPaths, filepath.ToSlash(relative))
			return nil
		}
		if err := copyReadOnlyFile(path, target); err != nil {
			return err
		}
		report.CopiedFiles++
		return nil
	})
	if err != nil {
		return SafeViewReport{}, fmt.Errorf("prepare safe read-only view: %w", err)
	}

	return report, nil
}

func IsSensitivePath(relativePath string) bool {
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
	switch base {
	case ".npmrc", ".pypirc", "netrc", ".netrc", "credentials", "credentials.json",
		"secrets", "secrets.json", "secrets.yaml", "secrets.yml",
		"id_rsa", "id_ed25519", "gradle.properties":
		return true
	}

	if strings.HasPrefix(base, "credentials.") || strings.HasPrefix(base, "secret.") || strings.HasPrefix(base, "secrets.") {
		return true
	}

	switch strings.ToLower(filepath.Ext(base)) {
	case ".pem", ".key", ".p12", ".pfx", ".jks", ".keystore":
		return true
	}

	return false
}

func copyReadOnlyFile(source, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}

	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()

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
