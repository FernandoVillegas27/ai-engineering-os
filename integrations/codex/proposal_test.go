package codex

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/proposal"
	"github.com/FernandoVillegas27/ai-engineering-os/integrations/commandmodel"
)

type proposalFakeRunner struct {
	invocation commandmodel.Invocation
}

func (f *proposalFakeRunner) LookPath(file string) (string, error) { return file, nil }

func (f *proposalFakeRunner) Run(_ context.Context, invocation commandmodel.Invocation) (string, string, error) {
	f.invocation = invocation
	var resultPath string
	for i := 0; i < len(invocation.Args)-1; i++ {
		if invocation.Args[i] == "--output-last-message" {
			resultPath = invocation.Args[i+1]
			break
		}
	}
	data, _ := json.Marshal(proposal.Proposal{
		Summary: "Agregar validación",
		Files: []proposal.FileChange{{
			Path: "service.go", Action: "modify", Rationale: "validar precio",
		}},
		Patch: "--- a/service.go\n+++ b/service.go",
		Risks: []proposal.Risk{{Severity: "low", Detail: "cambia validación"}},
		Verification: []string{"go test ./..."},
	})
	return "", "", os.WriteFile(resultPath, data, 0o600)
}

func proposalUnit(root string) execution.WorkUnit {
	return execution.WorkUnit{
		ID: "work-01-backend-engineer",
		Task: "Agregar validación",
		ProjectRoot: root,
		AgentID: "backend-engineer",
		Mode: execution.ModeReadOnly,
		Permissions: execution.PermissionEnvelope{
			SourceCode: "read", Tests: "read", GitCommit: "deny", GitPush: "deny", Secrets: "deny", Production: "deny",
		},
	}
}

func TestProposalRuntimeProducesDiffWithoutOriginalWrites(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "service.go")
	original := []byte("package service\n")
	if err := os.WriteFile(source, original, 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &proposalFakeRunner{}
	runtime := ProposalRuntime{Binary: "codex", Runner: runner}

	got, err := runtime.Propose(context.Background(), proposalUnit(root))
	if err != nil {
		t.Fatalf("Propose() error = %v", err)
	}
	after, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(original) {
		t.Fatal("original project was modified")
	}
	if got.Patch == "" || len(got.Files) != 1 {
		t.Fatalf("proposal = %#v", got)
	}
	if runner.invocation.Dir == root {
		t.Fatal("proposal runtime must use sanitized view")
	}
	if !strings.Contains(runner.invocation.Stdin, "PROPOSE") {
		t.Fatalf("prompt = %q", runner.invocation.Stdin)
	}
}

func TestProposalSchemaAvoidsUnsupportedKeywords(t *testing.T) {
	data, err := proposalSchema()
	if err != nil {
		t.Fatal(err)
	}
	for _, unsupported := range []string{"uniqueItems", "minItems", "maxItems"} {
		if strings.Contains(string(data), unsupported) {
			t.Fatalf("schema contains unsupported keyword %q", unsupported)
		}
	}
}
