package filekind

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/FernandoVillegas27/ai-engineering-os/core/staging"
)

type Kind string

const (
	KindUnknown Kind = "unknown"
	KindSource  Kind = "source"
	KindTest    Kind = "test"
)

func Classify(path string) (Kind, error) {
	normalized, err := normalize(path)
	if err != nil {
		return KindUnknown, err
	}
	parts := strings.Split(normalized, "/")
	base := strings.ToLower(parts[len(parts)-1])

	for _, part := range parts[:len(parts)-1] {
		switch strings.ToLower(part) {
		case "test", "tests", "__tests__", "spec", "specs", "integration-test", "integration-tests":
			return KindTest, nil
		}
	}

	switch {
	case strings.HasSuffix(base, "_test.go"):
		return KindTest, nil
	case strings.HasSuffix(base, "_test.py"), strings.HasPrefix(base, "test_") && strings.HasSuffix(base, ".py"):
		return KindTest, nil
	case hasAnySuffix(base,
		".test.ts", ".spec.ts", ".test.tsx", ".spec.tsx",
		".test.js", ".spec.js", ".test.jsx", ".spec.jsx",
		".test.mjs", ".spec.mjs", ".test.cjs", ".spec.cjs",
	):
		return KindTest, nil
	}

	ext := strings.ToLower(filepath.Ext(base))
	switch ext {
	case ".go", ".java", ".kt", ".kts", ".js", ".jsx", ".ts", ".tsx", ".py", ".cs", ".rb", ".php", ".rs", ".c", ".cc", ".cpp", ".h", ".hpp":
		return KindSource, nil
	default:
		return KindUnknown, nil
	}
}

func ValidateTestsOnly(changes []staging.WorkspaceChange) error {
	if len(changes) == 0 {
		return errors.New("tests-only scope requires at least one declared change")
	}
	for _, change := range changes {
		kind, err := Classify(change.Path)
		if err != nil {
			return err
		}
		if kind != KindTest {
			return fmt.Errorf("tests-only scope rejects %s: classified as %s", change.Path, kind)
		}
	}
	return nil
}

func normalize(value string) (string, error) {
	value = filepath.ToSlash(strings.TrimSpace(value))
	if value == "" {
		return "", errors.New("file-kind path is required")
	}
	if strings.HasPrefix(value, "/") || filepath.IsAbs(value) {
		return "", errors.New("file-kind path must be repository-relative")
	}
	clean := filepath.ToSlash(filepath.Clean(value))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", errors.New("file-kind path escapes repository")
	}
	if strings.ContainsRune(clean, '\x00') {
		return "", errors.New("file-kind path contains NUL")
	}
	return clean, nil
}

func hasAnySuffix(value string, suffixes ...string) bool {
	for _, suffix := range suffixes {
		if strings.HasSuffix(value, suffix) {
			return true
		}
	}
	return false
}
