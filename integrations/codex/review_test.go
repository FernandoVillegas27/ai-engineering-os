package codex

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestReviewTaskIncludesOriginalTaskAndFiles(t *testing.T) {
	text := reviewTask("Fix timeout", []string{"a.go", "b.go"})
	if text == "" || !containsAll(text, "Fix timeout", "a.go", "b.go") {
		t.Fatalf("reviewTask = %q", text)
	}
}

func TestReviewRuntimeConvertsBlockingFindings(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &executionFakeRunner{}
	runtime := ReviewRuntime{
		Execution: ExecutionRuntime{Binary: "codex", Runner: runner},
	}
	report, err := runtime.Review(context.Background(), root, "Review", []string{"main.go"})
	if err != nil {
		t.Fatal(err)
	}
	if string(report.Verdict) != "rejected" {
		t.Fatalf("report = %#v", report)
	}
}

func containsAll(value string, needles ...string) bool {
	for _, needle := range needles {
		if !strings.Contains(value, needle) {
			return false
		}
	}
	return true
}
