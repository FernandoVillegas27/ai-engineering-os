package codex

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FernandoVillegas27/ai-engineering-os/core/quality"
	"github.com/FernandoVillegas27/ai-engineering-os/core/review"
	"github.com/FernandoVillegas27/ai-engineering-os/core/verification"
	"github.com/FernandoVillegas27/ai-engineering-os/integrations/commandmodel"
)

type correctionFakeRunner struct {
	invocation commandmodel.Invocation
	extra      bool
}

func (f *correctionFakeRunner) LookPath(file string) (string, error) { return file, nil }

func (f *correctionFakeRunner) Run(_ context.Context, invocation commandmodel.Invocation) (string, string, error) {
	f.invocation = invocation
	if err := os.WriteFile(filepath.Join(invocation.Dir, "service.go"), []byte("package sample\nfunc Value() int { return 2 }\n"), 0o644); err != nil {
		return "", "", err
	}
	if f.extra {
		if err := os.WriteFile(filepath.Join(invocation.Dir, "extra.go"), []byte("package sample\n"), 0o644); err != nil {
			return "", "", err
		}
	}
	return "", "", nil
}

func TestCorrectionRuntimeChangesAllowedFile(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "service.go"), []byte("package sample\nfunc Value() int { return 1 }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &correctionFakeRunner{}
	runtime := CorrectionRuntime{Binary: "codex", Runner: runner}
	err := runtime.Correct(context.Background(), quality.CorrectionRequest{
		Workspace:    root,
		Task:         "Fix value",
		AllowedFiles: []string{"service.go"},
		Verification: verification.Report{Status: verification.StatusFailed},
		Review:       review.Report{Verdict: review.VerdictRejected},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(runner.invocation.Args, " "), "--sandbox workspace-write") {
		t.Fatalf("args = %#v", runner.invocation.Args)
	}
}

func TestCorrectionRuntimeRejectsUndeclaredChange(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "service.go"), []byte("package sample\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := (CorrectionRuntime{Binary: "codex", Runner: &correctionFakeRunner{extra: true}}).Correct(
		context.Background(),
		quality.CorrectionRequest{
			Workspace:    root,
			Task:         "Fix",
			AllowedFiles: []string{"service.go"},
		},
	)
	if err == nil {
		t.Fatal("expected undeclared correction change rejection")
	}
}
