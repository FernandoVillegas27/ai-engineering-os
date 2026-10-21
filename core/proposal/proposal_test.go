package proposal

import (
	"context"
	"fmt"
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


func TestGeneratorRejectsRealCursorRuntimeSelectorDrift(t *testing.T) {
	unit := validUnit()
	unit.Task = "Agregá una validación para que el timeout del runtime Codex no pueda configurarse en menos de 5 segundos"
	runtime := &fakeRuntime{proposal: Proposal{
		Summary: "Alinear ai propose con el selector de runtime auto, codex y cursor, igual que ai stage.",
		Files: []FileChange{
			{Path: "cmd/ai/propose.go", Action: "modify", Rationale: "Permitir --runtime auto|codex|cursor; en auto usar runtimefallback.ProposalRuntime con Codex primario y Cursor como fallback"},
			{Path: "cmd/ai/propose_test.go", Action: "modify", Rationale: "Cubrir parseo de auto/cursor y rechazo de runtimes inválidos; ajustar el default si pasa a auto"},
			{Path: "docs/propose-changes.md", Action: "modify", Rationale: "Documentar los runtimes soportados y que Patch puede quedar vacío con Cursor"},
			{Path: "docs/runtime-fallback.md", Action: "modify", Rationale: "Confirmar que el flujo de proposal Codex→Cursor también aplica a ai propose"},
		},
		Verification: []string{
			"go test ./cmd/ai/...",
			"go test ./core/runtimefallback/...",
			"go run ./cmd/ai propose . --task \"Agregar validación de precios negativos\" --runtime auto",
			"go run ./cmd/ai propose . --task \"Agregar validación de precios negativos\" --runtime cursor",
		},
	}}

	_, err := (Generator{Runtime: runtime}).Generate(context.Background(), unit)
	if err == nil {
		t.Fatal("expected real Cursor drift proposal to be rejected")
	}
}

func TestValidateTaskAlignmentRequiresExplicitNumericConstraint(t *testing.T) {
	task := "Set the timeout to at least 5 seconds"
	p := Proposal{
		Summary: "Set the timeout to a safer minimum.",
		Files: []FileChange{{Path: "timeout.go", Action: "modify", Rationale: "Validate timeout configuration"}},
	}
	if err := ValidateTaskAlignment(task, p); err == nil {
		t.Fatal("expected missing numeric constraint to be rejected")
	}
}


func TestIsTaskAlignmentErrorRecognizesWrappedAlignmentFailure(t *testing.T) {
	err := fmt.Errorf("proposal failed: %w", TaskAlignmentError{Detail: "missing timeout"})
	if !IsTaskAlignmentError(err) {
		t.Fatal("expected wrapped task alignment error")
	}
}

func TestIsTaskAlignmentErrorRejectsOtherErrors(t *testing.T) {
	if IsTaskAlignmentError(fmt.Errorf("other failure")) {
		t.Fatal("unexpected task alignment classification")
	}
}
