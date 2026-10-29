package cursor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	goruntime "runtime"
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
	runner := &fakeRunner{stdout: `{"type":"system","subtype":"init"}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"{\"Summary\":\"Actualizar\",\"Files\":[{\"Path\":\"service.go\",\"Action\":\"modify\",\"Rationale\":\"pedido\"}],\"Patch\":\"\",\"Risks\":[],\"Verification\":[\"go test ./...\"]}"}]}}
{"type":"result","subtype":"success","is_error":false,"result":"{\"Summary\":\"Actualizar\",\"Files\":[{\"Path\":\"service.go\",\"Action\":\"modify\",\"Rationale\":\"pedido\"}],\"Patch\":\"\",\"Risks\":[],\"Verification\":[\"go test ./...\"]}"}`}
	runtime := ProposalRuntime{Config: RunnerConfig{Binary: "agent", Runner: runner}}

	got, err := runtime.Propose(context.Background(), unitFixture(root))
	if err != nil {
		t.Fatal(err)
	}
	if got.Summary != "Actualizar" || len(got.Files) != 1 {
		t.Fatalf("proposal=%#v", got)
	}
	args := strings.Join(runner.invocations[0].Args, " ")
	if !strings.Contains(args, "--mode ask") || !strings.Contains(args, "--sandbox "+cursorSandboxMode(goruntime.GOOS)) || !strings.Contains(args, "--output-format stream-json") || !strings.Contains(args, "--trust") {
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
	if !strings.Contains(args, "--sandbox "+cursorSandboxMode(goruntime.GOOS)) || !strings.Contains(args, "--workspace "+result.Workspace) || !strings.Contains(args, "--trust") {
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
	runner := &fakeRunner{stdout: `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"{\"Summary\":\"Revisión\",\"Findings\":[{\"Severity\":\"high\",\"Title\":\"Bug\",\"Detail\":\"detalle\"}]}"}]}}
{"type":"result","subtype":"success","is_error":false,"result":"{\"Summary\":\"Revisión\",\"Findings\":[{\"Severity\":\"high\",\"Title\":\"Bug\",\"Detail\":\"detalle\"}]}"}`}
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


func TestCursorRuntimeConstructorsAcceptExplicitModel(t *testing.T) {
	const model = "gpt-5.6-sol-high"
	if got := NewProposalRuntimeWithModel(model).Config.Model; got != model {
		t.Fatalf("proposal model=%q", got)
	}
	if got := NewStageRuntimeWithModel(model).Config.Model; got != model {
		t.Fatalf("stage model=%q", got)
	}
	if got := NewReviewRuntimeWithModel(model).Config.Model; got != model {
		t.Fatalf("review model=%q", got)
	}
	if got := NewCorrectionRuntimeWithModel(model).Config.Model; got != model {
		t.Fatalf("correction model=%q", got)
	}
}


func TestStageRuntimeStillUsesTextOutput(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "service.go"), []byte("package sample\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{writeFile: "service.go", stdout: "done"}
	result, err := (StageRuntime{Config: RunnerConfig{Runner: runner}}).Stage(context.Background(), unitFixture(root), proposal.Proposal{
		Summary: "Actualizar",
		Files: []proposal.FileChange{{Path: "service.go", Action: "modify"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(result.Workspace) })
	args := strings.Join(runner.invocations[0].Args, " ")
	if !strings.Contains(args, "--output-format text") {
		t.Fatalf("args=%q", args)
	}
}


func TestDecodeStreamFinalResponsePrefersTerminalResult(t *testing.T) {
	raw := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"partial"}]}}
{"type":"result","subtype":"success","is_error":false,"result":"final response"}`
	got, err := decodeStreamFinalResponse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got != "final response" {
		t.Fatalf("got=%q", got)
	}
}

func TestDecodeStreamFinalResponseFallsBackToAssistantDeltas(t *testing.T) {
	raw := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"hello "}]}}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"world"}]}}
{"type":"result","subtype":"success","is_error":false,"result":""}`
	got, err := decodeStreamFinalResponse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got != "hello world" {
		t.Fatalf("got=%q", got)
	}
}

func TestDecodeStreamFinalResponseRequiresTerminalResult(t *testing.T) {
	raw := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"hello"}]}}`
	if _, err := decodeStreamFinalResponse(raw); err == nil {
		t.Fatal("expected missing terminal result error")
	}
}

func TestDecodeStreamFinalResponseRejectsTerminalError(t *testing.T) {
	raw := `{"type":"result","subtype":"error","is_error":true,"result":"failed"}`
	if _, err := decodeStreamFinalResponse(raw); err == nil {
		t.Fatal("expected terminal error")
	}
}

func TestDecodeModelJSONObjectIncludesResponseSampleOnFailure(t *testing.T) {
	var got proposal.Proposal
	err := decodeModelJSONObject("not json at all", &got)
	if err == nil || !strings.Contains(err.Error(), "not json at all") {
		t.Fatalf("err=%v", err)
	}
}

func TestRunAgentWithFormatUsesStreamJSON(t *testing.T) {
	root := t.TempDir()
	runner := &fakeRunner{stdout: `{"type":"result","subtype":"success","is_error":false,"result":"ok"}`}
	_, err := runAgentWithFormat(context.Background(), RunnerConfig{Runner: runner}, root, "ask", "test", "stream-json")
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Join(runner.invocations[0].Args, " ")
	if !strings.Contains(args, "--output-format stream-json") {
		t.Fatalf("args=%q", args)
	}
}

func TestProposalPassesFullTaskPromptAsSingleArgument(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "service.go"), []byte("package sample\n"), 0o600); err != nil { t.Fatal(err) }
	runner := &fakeRunner{stdout: `{"type":"result","subtype":"success","result":"{\"Summary\":\"OK\",\"Files\":[],\"Patch\":\"\",\"Risks\":[],\"Verification\":[]}" }`}
	unit := unitFixture(root)
	unit.Task = "Validar que Codex timeout no sea menor a 5 segundos"
	_, err := (ProposalRuntime{Config: RunnerConfig{Runner:runner}}).Propose(context.Background(), unit)
	if err != nil { t.Fatal(err) }
	args := runner.invocations[0].Args
	if len(args) < 2 { t.Fatal("missing arguments") }
	prompt := args[len(args)-1]
	if !strings.Contains(prompt, unit.Task) || strings.Count(prompt, unit.Task) < 2 { t.Fatal("original task missing from CLI prompt argument") }
	if !strings.Contains(prompt, "Return EXACTLY one JSON object") { t.Fatal("JSON contract missing from CLI prompt") }
}

func TestProposalOptionalDiagnosticsDoNotLogPrompt(t *testing.T) {
	t.Setenv("AI_CURSOR_DIAGNOSTICS", "1")
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "service.go"), []byte("package sample\n"), 0o600); err != nil { t.Fatal(err) }
	runner := &fakeRunner{stdout: `{"type":"result","subtype":"success","result":"Please specify the task"}`}
	unit := unitFixture(root)
	unit.Task = "Sensitive test marker that must not be in diagnostics"
	_, err := (ProposalRuntime{Config: RunnerConfig{Runner:runner}}).Propose(context.Background(), unit)
	if err == nil { t.Fatal("expected non-JSON response error") }
	message := err.Error()
	for _, marker := range []string{"prompt_bytes=", "prompt_sha256=", "task_in_prompt=true", "sanitized_files="} {
		if !strings.Contains(message, marker) { t.Fatalf("missing %q in %s", marker, message) }
	}
	if strings.Contains(message, unit.Task) { t.Fatal("diagnostics leaked the prompt") }
}
