package codex

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/orchestration"
	"github.com/FernandoVillegas27/ai-engineering-os/integrations/commandmodel"
)

type executionFakeRunner struct {
	invocation      commandmodel.Invocation
	sensitiveExists bool
}

func (f *executionFakeRunner) LookPath(file string) (string, error) {
	return file, nil
}

func (f *executionFakeRunner) Run(_ context.Context, invocation commandmodel.Invocation) (string, string, error) {
	f.invocation = invocation
	_, envErr := os.Stat(filepath.Join(invocation.Dir, ".env"))
	f.sensitiveExists = envErr == nil

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
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("TOKEN=secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &executionFakeRunner{}
	runtime := ExecutionRuntime{Binary: "codex", Runner: runner}

	result, err := runtime.Analyze(context.Background(), executionUnitFixture(root))
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}

	if runner.invocation.Dir == root {
		t.Fatal("Codex must not run against the original project root")
	}
	projectParent := filepath.Dir(root)
	runtimeWorkspace := filepath.Dir(runner.invocation.Dir)
	if filepath.Dir(runtimeWorkspace) != projectParent {
		t.Fatalf("runtime workspace %q is not beside project parent %q", runtimeWorkspace, projectParent)
	}
	if runner.sensitiveExists {
		t.Fatal("sensitive .env file was visible to Codex runtime")
	}
	for _, entry := range runner.invocation.Env {
		key := strings.SplitN(entry, "=", 2)[0]
		upper := strings.ToUpper(key)
		if strings.Contains(upper, "TOKEN") ||
			strings.Contains(upper, "SECRET") ||
			strings.Contains(upper, "PASSWORD") ||
			strings.Contains(upper, "API_KEY") {
			t.Fatalf("sensitive environment variable leaked to Codex: %s", key)
		}
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
	if !strings.Contains(runner.invocation.Stdin, "READ-ONLY") ||
		!strings.Contains(runner.invocation.Stdin, "sanitized project view") {
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


func TestSafeRuntimeEnvironmentDoesNotPassArbitrarySecrets(t *testing.T) {
	t.Setenv("AI_ENGINEERING_OS_TEST_SECRET_TOKEN", "do-not-leak")
	t.Setenv("OPENAI_API_KEY", "do-not-leak")

	env := safeRuntimeEnvironment()
	for _, entry := range env {
		if strings.HasPrefix(entry, "AI_ENGINEERING_OS_TEST_SECRET_TOKEN=") ||
			strings.HasPrefix(entry, "OPENAI_API_KEY=") {
			t.Fatalf("secret environment variable leaked: %s", entry)
		}
	}
}


type blockingExecutionRunner struct{}

func (blockingExecutionRunner) LookPath(file string) (string, error) {
	return file, nil
}

func (blockingExecutionRunner) Run(ctx context.Context, _ commandmodel.Invocation) (string, string, error) {
	<-ctx.Done()
	return "", "", ctx.Err()
}

func TestExecutionRuntimeTimesOut(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main"), 0o600); err != nil {
		t.Fatal(err)
	}

	runtime := ExecutionRuntime{
		Binary:  "codex",
		Runner:  blockingExecutionRunner{},
		Timeout: 20 * time.Millisecond,
	}

	_, err := runtime.Analyze(context.Background(), executionUnitFixture(root))
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected timeout error, got %v", err)
	}
}

func TestSafeRuntimeEnvironmentPreservesCodexHomePathOnly(t *testing.T) {
	t.Setenv("CODEX_HOME", `C:\\Users\\Fernando\\.codex`)
	t.Setenv("OPENAI_API_KEY", "do-not-leak")

	env := safeRuntimeEnvironment()
	joined := strings.Join(env, "
")
	if !strings.Contains(joined, "CODEX_HOME=") {
		t.Fatal("expected CODEX_HOME to be preserved")
	}
	if strings.Contains(joined, "OPENAI_API_KEY=") {
		t.Fatal("OPENAI_API_KEY must not be forwarded")
	}
}
