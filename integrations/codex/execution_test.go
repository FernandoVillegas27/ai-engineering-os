package codex

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/orchestration"
	"github.com/FernandoVillegas27/ai-engineering-os/integrations/commandmodel"
)

type executionFakeRunner struct {
	invocation commandmodel.Invocation
}

func (f *executionFakeRunner) LookPath(file string) (string, error) {
	return file, nil
}

func (f *executionFakeRunner) Run(_ context.Context, invocation commandmodel.Invocation) (string, string, error) {
	f.invocation = invocation

	var resultPath string
	for i := 0; i < len(invocation.Args)-1; i++ {
		if invocation.Args[i] == "--output-last-message" {
			resultPath = invocation.Args[i+1]
			break
		}
	}
	if resultPath == "" {
		return "", "", os.ErrInvalid
	}

	data, _ := json.Marshal(execution.Analysis{
		Summary: "reviewed",
		Findings: []execution.Finding{{
			Severity: "medium",
			Title:    "Missing permission check",
			Detail:   "Update path lacks an explicit authorization guard.",
		}},
		Evidence: []execution.Evidence{{
			Path:   "core/handler.go",
			Detail: "Update occurs before authorization verification.",
		}},
	})
	if err := os.WriteFile(resultPath, data, 0o600); err != nil {
		return "", "", err
	}
	return "", "", nil
}

func executionUnitFixture(root string) execution.WorkUnit {
	return execution.WorkUnit{
		ID:           "work-01-reviewer",
		Task:         "Review permission handling",
		ProjectRoot:  root,
		Intent:       orchestration.IntentReview,
		AgentID:      "reviewer",
		Capabilities: []string{"code-review", "security-review"},
		Skills:       []string{"security-review-skill"},
		Tools:        []string{"filesystem"},
		Mode:         execution.ModeReadOnly,
		Permissions: execution.PermissionEnvelope{
			SourceCode: "read",
			Tests:      "read",
			GitCommit:  "deny",
			GitPush:    "deny",
			Secrets:    "deny",
			Production: "deny",
		},
	}
}

func TestExecutionRuntimeRunsCodexReadOnlyInProject(t *testing.T) {
	root := t.TempDir()
	runner := &executionFakeRunner{}
	runtime := ExecutionRuntime{Binary: "codex", Runner: runner}

	result, err := runtime.Analyze(context.Background(), executionUnitFixture(root))
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}

	if runner.invocation.Dir != root {
		t.Fatalf("dir = %q, want %q", runner.invocation.Dir, root)
	}
	joined := strings.Join(runner.invocation.Args, " ")
	for _, expected := range []string{
		"exec",
		"--ephemeral",
		"--sandbox read-only",
		`-c approval_policy="never"`,
		"--output-schema",
		"--output-last-message",
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("args %q missing %q", joined, expected)
		}
	}
	if !strings.Contains(runner.invocation.Stdin, "READ-ONLY") {
		t.Fatalf("prompt = %q", runner.invocation.Stdin)
	}
	if len(result.Findings) != 1 || len(result.Evidence) != 1 {
		t.Fatalf("result = %#v", result)
	}
}

func TestExecutionSchemaAvoidsUnsupportedKeywords(t *testing.T) {
	data, err := executionSchema()
	if err != nil {
		t.Fatalf("executionSchema() error = %v", err)
	}
	text := string(data)
	for _, unsupported := range []string{"uniqueItems", "minItems", "maxItems"} {
		if strings.Contains(text, unsupported) {
			t.Fatalf("schema contains unsupported keyword %q: %s", unsupported, text)
		}
	}
}

func TestExecutionRuntimeRejectsNonReadOnlyUnit(t *testing.T) {
	unit := executionUnitFixture(filepath.Clean("."))
	unit.Mode = execution.Mode("write")

	_, err := (ExecutionRuntime{}).Analyze(context.Background(), unit)
	if err == nil {
		t.Fatal("expected read-only mode error")
	}
}
