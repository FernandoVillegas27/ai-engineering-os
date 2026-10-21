package cursor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/proposal"
	"github.com/FernandoVillegas27/ai-engineering-os/core/quality"
	"github.com/FernandoVillegas27/ai-engineering-os/core/review"
	"github.com/FernandoVillegas27/ai-engineering-os/integrations/commandmodel"
)

type fakeRunner struct {
	invocations []commandmodel.Invocation
	stdout      string
	stderr      string
	err         error
	writeFile   string
	writeExtra  bool
}

func (f *fakeRunner) LookPath(file string) (string, error) { return file, nil }

func (f *fakeRunner) Run(_ context.Context, invocation commandmodel.Invocation) (string, string, error) {
	f.invocations = append(f.invocations, invocation)
	if f.writeFile != "" {
		target := filepath.Join(invocation.Dir, filepath.FromSlash(f.writeFile))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return "", "", err
		}
		if err := os.WriteFile(target, []byte("package sample\nfunc Value() int { return 2 }\n"), 0o644); err != nil {
			return "", "", err
		}
	}
	if f.writeExtra {
		if err := os.WriteFile(filepath.Join(invocation.Dir, "extra.go"), []byte("package sample\n"), 0o644); err != nil {
			return "", "", err
		}
	}
	return f.stdout, f.stderr, f.err
}

func unitFixture(root string) execution.WorkUnit {
	return execution.WorkUnit{
		ID:          "work-01-backend-engineer",
		Task:        "Actualizar valor",
		ProjectRoot: root,
		AgentID:     "backend-engineer",
		Mode:        execution.ModeReadOnly,
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

func TestProposalRuntimeUsesReadOnlyAskModeAndParsesJSON(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "service.go"), []byte("package sample\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{stdout: `{"Summary":"Actualizar","Files":[{"Path":"service.go","Action":"modify","Rationale":"pedido"}],"Patch":"","Risks":[],"Verification":["go test ./..."]}`}
	runtime := ProposalRuntime{Config: RunnerConfig{Binary: "agent", Runner: runner}}

	got, err := runtime.Propose(context.Background(), unitFixture(root))
	if err != nil {
		t.Fatal(err)
	}
	if got.Summary != "Actualizar" || len(got.Files) != 1 {
		t.Fatalf("proposal=%#v", got)
	}
	args := strings.Join(runner.invocations[0].Args, " ")
	if !strings.Contains(args, "--mode ask") || !strings.Contains(args, "--sandbox enabled") || !strings.Contains(args, "--output-format text") || !strings.Contains(args, "--trust") {
		t.Fatalf("args=%q", args)
	}
	if runner.invocations[0].Dir == root {
		t.Fatal("proposal must use sanitized view")
	}
}

func TestStageRuntimeWritesOnlyIsolatedWorkspace(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "service.go")
	original := []byte("package sample\nfunc Value() int { return 1 }\n")
	if err := os.WriteFile(source, original, 0o600); err != nil {
		t.Fatal(err)
	}

	runner := &fakeRunner{writeFile: "service.go", stdout: "done"}
	runtime := StageRuntime{Config: RunnerConfig{Binary: "agent", Runner: runner}}
	p := proposal.Proposal{
		Summary: "Actualizar",
		Files: []proposal.FileChange{{Path: "service.go", Action: "modify", Rationale: "pedido"}},
	}

	result, err := runtime.Stage(context.Background(), unitFixture(root), p)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(result.Workspace) })

	after, _ := os.ReadFile(source)
	if string(after) != string(original) {
		t.Fatal("original project changed")
	}
	if len(result.Changes) != 1 || result.Changes[0].Path != "service.go" {
		t.Fatalf("changes=%#v", result.Changes)
	}
	args := strings.Join(runner.invocations[0].Args, " ")
	if !strings.Contains(args, "--sandbox enabled") || !strings.Contains(args, "--workspace "+result.Workspace) || !strings.Contains(args, "--trust") {
		t.Fatalf("args=%q", args)
	}
}

func TestStageRuntimeRejectsUndeclaredChange(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "service.go"), []byte("package sample\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{writeFile: "service.go", writeExtra: true}
	_, err := (StageRuntime{Config: RunnerConfig{Runner: runner}}).Stage(context.Background(), unitFixture(root), proposal.Proposal{
		Summary: "Actualizar",
		Files: []proposal.FileChange{{Path: "service.go", Action: "modify"}},
	})
	if err == nil {
		t.Fatal("expected undeclared change rejection")
	}
}

func TestReviewRuntimeConvertsBlockingFinding(t *testing.T) {
	root := t.TempDir()
	runner := &fakeRunner{stdout: `{"Summary":"Revisión","Findings":[{"Severity":"high","Title":"Bug","Detail":"detalle"}]}`}
	report, err := (ReviewRuntime{Config: RunnerConfig{Runner: runner}}).Review(context.Background(), root, "task", []string{"service.go"})
	if err != nil {
		t.Fatal(err)
	}
	if report.Verdict != review.VerdictRejected {
		t.Fatalf("report=%#v", report)
	}
}

func TestCorrectionRuntimeRejectsUndeclaredFile(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "service.go"), []byte("package sample\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{writeFile: "service.go", writeExtra: true}
	err := (CorrectionRuntime{Config: RunnerConfig{Runner: runner}}).Correct(context.Background(), quality.CorrectionRequest{
		Workspace:    root,
		Task:         "fix",
		AllowedFiles: []string{"service.go"},
	})
	if err == nil {
		t.Fatal("expected undeclared correction rejection")
	}
}

func TestRunAgentPropagatesProviderError(t *testing.T) {
	root := t.TempDir()
	runner := &fakeRunner{stderr: "usage limit reached", err: errors.New("exit 1")}
	_, err := runAgent(context.Background(), RunnerConfig{Runner: runner}, root, "ask", "test")
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "usage limit") {
		t.Fatalf("err=%v", err)
	}
}


func TestCursorSandboxModeUsesAllowlistOnWindows(t *testing.T) {
	if got := cursorSandboxMode("windows"); got != "disabled" {
		t.Fatalf("cursorSandboxMode(windows) = %q, want disabled", got)
	}
}

func TestCursorSandboxModeKeepsSandboxOnUnixPlatforms(t *testing.T) {
	for _, goos := range []string{"linux", "darwin"} {
		if got := cursorSandboxMode(goos); got != "enabled" {
			t.Fatalf("cursorSandboxMode(%s) = %q, want enabled", goos, got)
		}
	}
}


func TestProposalPromptStronglyGroundsPrimaryTask(t *testing.T) {
	unit := unitFixture("C:/project")
	unit.Task = "Agregá una validación para que el timeout del runtime Codex no pueda configurarse en menos de 5 segundos"
	prompt := proposalPrompt(unit, execution.SafeViewReport{CopiedFiles: 42})

	required := []string{
		"PRIMARY TASK",
		unit.Task,
		"the only requested objective",
		"preserve every explicit numeric constraint",
		"named technical subject",
		"Every file Rationale MUST explain exactly how that file contributes to the PRIMARY TASK",
		"Verification steps MUST verify the requested behavior itself",
		"Ignore TODOs, recent-feature themes, docs, or nearby code that suggest a different task",
		"Never substitute a different task",
	}
	for _, expected := range required {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("proposal prompt missing %q\n%s", expected, prompt)
		}
	}
}

func TestProposalPromptRepeatsTaskForGrounding(t *testing.T) {
	unit := unitFixture("C:/project")
	unit.Task = "Keep Codex timeout above 5 seconds"
	prompt := proposalPrompt(unit, execution.SafeViewReport{})
	if count := strings.Count(prompt, unit.Task); count < 2 {
		t.Fatalf("task occurrence count=%d, want at least 2", count)
	}
}


func TestCursorRuntimeConstructorsPinExplicitModels(t *testing.T) {
	if got := NewProposalRuntime().Config.Model; got != defaultEngineeringModel {
		t.Fatalf("proposal model=%q want=%q", got, defaultEngineeringModel)
	}
	if got := NewStageRuntime().Config.Model; got != defaultEngineeringModel {
		t.Fatalf("stage model=%q want=%q", got, defaultEngineeringModel)
	}
	if got := NewCorrectionRuntime().Config.Model; got != defaultEngineeringModel {
		t.Fatalf("correction model=%q want=%q", got, defaultEngineeringModel)
	}
	if got := NewReviewRuntime().Config.Model; got != defaultReviewModel {
		t.Fatalf("review model=%q want=%q", got, defaultReviewModel)
	}
}

func TestRunAgentPassesConfiguredModel(t *testing.T) {
	root := t.TempDir()
	runner := &fakeRunner{stdout: `{"Summary":"ok","Findings":[]}`}
	_, err := runAgent(context.Background(), RunnerConfig{
		Runner: runner,
		Model:  defaultEngineeringModel,
	}, root, "ask", "test")
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Join(runner.invocations[0].Args, " ")
	if !strings.Contains(args, "--model "+defaultEngineeringModel) {
		t.Fatalf("args=%q", args)
	}
}
