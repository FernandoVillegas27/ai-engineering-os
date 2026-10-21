package gitstage

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FernandoVillegas27/ai-engineering-os/core/proposal"
	"github.com/FernandoVillegas27/ai-engineering-os/integrations/commandmodel"
)

type fakeRunner struct {
	invocations []commandmodel.Invocation
}

func (f *fakeRunner) LookPath(file string) (string, error) { return file, nil }

func (f *fakeRunner) Run(_ context.Context, invocation commandmodel.Invocation) (string, string, error) {
	f.invocations = append(f.invocations, invocation)
	target := filepath.Join(invocation.Dir, "src", "service.go")
	if len(invocation.Args) > 1 && invocation.Args[1] == "--check" {
		return "", "", nil
	}
	return "", "", os.WriteFile(target, []byte("package src\nfunc Value() int { return 2 }\n"), 0o644)
}

func TestStageUsesIsolatedWorkspaceAndPreservesOriginal(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	original := []byte("package src\nfunc Value() int { return 1 }\n")
	source := filepath.Join(root, "src", "service.go")
	if err := os.WriteFile(source, original, 0o600); err != nil {
		t.Fatal(err)
	}

	p := proposal.Proposal{
		Files: []proposal.FileChange{{Path: "src/service.go", Action: "modify"}},
		Patch: strings.Join([]string{
			"diff --git a/src/service.go b/src/service.go",
			"--- a/src/service.go",
			"+++ b/src/service.go",
			"@@ -1,2 +1,2 @@",
			" package src",
			"-func Value() int { return 1 }",
			"+func Value() int { return 2 }",
		}, "\n"),
	}

	runner := &fakeRunner{}
	result, err := (Stager{Runner: runner}).Stage(context.Background(), root, p)
	if err != nil {
		t.Fatalf("Stage() error = %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(result.Workspace) })

	after, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(original) {
		t.Fatal("original project changed")
	}
	staged, err := os.ReadFile(filepath.Join(result.Workspace, "src", "service.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(staged), "return 2") {
		t.Fatalf("staged file = %s", staged)
	}
	if result.Workspace == root || filepath.Dir(result.Workspace) != filepath.Dir(root) {
		t.Fatalf("workspace = %q, root = %q", result.Workspace, root)
	}
	if len(runner.invocations) != 2 {
		t.Fatalf("invocations = %d, want 2", len(runner.invocations))
	}
	for _, invocation := range runner.invocations {
		for _, env := range invocation.Env {
			upper := strings.ToUpper(env)
			if strings.Contains(upper, "TOKEN=") || strings.Contains(upper, "SECRET=") || strings.Contains(upper, "PASSWORD=") || strings.Contains(upper, "API_KEY=") {
				t.Fatalf("sensitive environment leaked: %s", env)
			}
		}
	}
}


func TestStageHandlesWindowsCRLFContextWithLFPatch(t *testing.T) {
	gitPath, err := commandmodel.OSRunner{}.LookPath("git")
	if err != nil {
		t.Skip("git not available")
	}

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "src", "service.go")
	original := []byte("package src\r\n\r\nfunc Value() int {\r\n\treturn 1\r\n}\r\n")
	if err := os.WriteFile(source, original, 0o600); err != nil {
		t.Fatal(err)
	}

	p := proposal.Proposal{
		Files: []proposal.FileChange{{Path: "src/service.go", Action: "modify"}},
		Patch: strings.Join([]string{
			"diff --git a/src/service.go b/src/service.go",
			"--- a/src/service.go",
			"+++ b/src/service.go",
			"@@ -1,5 +1,5 @@",
			" package src",
			" ",
			" func Value() int {",
			"-\treturn 1",
			"+\treturn 2",
			" }",
		}, "\n"),
	}

	result, err := (Stager{Runner: commandmodel.OSRunner{}}).Stage(context.Background(), root, p)
	if err != nil {
		t.Fatalf("Stage() with CRLF source error = %v (git=%s)", err, gitPath)
	}
	t.Cleanup(func() { _ = os.RemoveAll(result.Workspace) })

	afterOriginal, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if string(afterOriginal) != string(original) {
		t.Fatal("original CRLF project changed")
	}

	staged, err := os.ReadFile(filepath.Join(result.Workspace, "src", "service.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(staged), "return 2") {
		t.Fatalf("staged file was not patched: %q", staged)
	}
}


func TestStageNormalizesMissingPatchTrailingNewline(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "service.go"), []byte("package src\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	p := proposal.Proposal{
		Files: []proposal.FileChange{{Path: "src/service.go", Action: "modify"}},
		Patch: strings.Join([]string{
			"diff --git a/src/service.go b/src/service.go",
			"--- a/src/service.go",
			"+++ b/src/service.go",
			"@@ -1 +1 @@",
			"-package src",
			"+package changed",
		}, "\n"),
	}

	runner := &fakeRunner{}
	result, err := (Stager{Runner: runner}).Stage(context.Background(), root, p)
	if err != nil {
		t.Fatalf("Stage() error = %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(result.Workspace) })

	for _, invocation := range runner.invocations {
		if !strings.HasSuffix(invocation.Stdin, "\n") {
			t.Fatal("git apply input must end with a newline")
		}
	}
}
