package proposal

import (
	"context"
	"testing"

	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/orchestration"
)

type fakeRuntime struct {
	called   bool
	proposal Proposal
}

func (f *fakeRuntime) ID() string { return "fake" }

func (f *fakeRuntime) Propose(_ context.Context, _ execution.WorkUnit) (Proposal, error) {
	f.called = true
	return f.proposal, nil
}

func validUnit() execution.WorkUnit {
	return execution.WorkUnit{
		ID:      "work-01-backend-engineer",
		AgentID: "backend-engineer",
		Mode:    execution.ModeReadOnly,
		Intent:  orchestration.IntentImplement,
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

func TestGeneratorReturnsStructuredProposal(t *testing.T) {
	runtime := &fakeRuntime{proposal: Proposal{
		Summary: "Add validation",
		Files: []FileChange{{Path: "service.go", Action: "modify", Rationale: "validate input"}},
		Patch: "--- a/service.go\n+++ b/service.go",
		Risks: []Risk{{Severity: "low", Detail: "behavior change"}},
		Verification: []string{"go test ./..."},
	}}

	got, err := (Generator{Runtime: runtime}).Generate(context.Background(), validUnit())
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if !runtime.called || got.Summary != "Add validation" || len(got.Files) != 1 {
		t.Fatalf("proposal = %#v", got)
	}
}

func TestGeneratorRejectsInvalidPermissionsBeforeRuntime(t *testing.T) {
	unit := validUnit()
	unit.Permissions.Secrets = "read"
	runtime := &fakeRuntime{proposal: Proposal{Summary: "must not run"}}

	_, err := (Generator{Runtime: runtime}).Generate(context.Background(), unit)
	if err == nil {
		t.Fatal("expected permission validation error")
	}
	if runtime.called {
		t.Fatal("runtime must not be called")
	}
}


func TestGeneratorRejectsTaskMisalignedProposal(t *testing.T) {
	unit := validUnit()
	unit.Task = "Agregá una validación para que el timeout del runtime Codex no pueda configurarse en menos de 5 segundos"
	runtime := &fakeRuntime{proposal: Proposal{
		Summary: "Alinear ai propose con el selector de runtime auto, codex y cursor",
		Files: []FileChange{
			{Path: "cmd/ai/propose.go", Action: "modify", Rationale: "Agregar selector de runtime"},
			{Path: "docs/runtime-fallback.md", Action: "modify", Rationale: "Documentar fallback"},
		},
		Verification: []string{"go test ./cmd/ai/..."},
	}}

	_, err := (Generator{Runtime: runtime}).Generate(context.Background(), unit)
	if err == nil {
		t.Fatal("expected task alignment rejection")
	}
}

func TestGeneratorAcceptsTaskAlignedProposal(t *testing.T) {
	unit := validUnit()
	unit.Task = "Agregá una validación para que el timeout del runtime Codex no pueda configurarse en menos de 5 segundos"
	runtime := &fakeRuntime{proposal: Proposal{
		Summary: "Validar que el timeout del runtime Codex sea cero o al menos 5 segundos.",
		Files: []FileChange{
			{Path: "integrations/codex/timeout.go", Action: "modify", Rationale: "Centralizar el mínimo de timeout de Codex"},
		},
		Verification: []string{"go test ./integrations/codex/..."},
	}}

	got, err := (Generator{Runtime: runtime}).Generate(context.Background(), unit)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if got.Summary == "" {
		t.Fatal("expected proposal")
	}
}
