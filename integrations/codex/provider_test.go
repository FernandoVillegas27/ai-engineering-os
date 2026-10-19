package codex

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/FernandoVillegas27/ai-engineering-os/core/modelprovider"
	"github.com/FernandoVillegas27/ai-engineering-os/core/orchestration"
)

func requestFixture() modelprovider.ClassificationRequest {
	return modelprovider.ClassificationRequest{
		Task: "Necesito mejorar esto",
		ProjectContext: "Angular project",
		AllowedIntents: []orchestration.Intent{
			orchestration.IntentImplement,
			orchestration.IntentReview,
		},
		AllowedCapabilities: []modelprovider.CapabilityOption{
			{
				ID:          "frontend-development",
				Description: "Build and maintain user-facing web application interfaces.",
				Aliases:     []string{"frontend", "ui-development"},
			},
			{
				ID:          "code-review",
				Description: "Review code for correctness and maintainability.",
				Aliases:     []string{"review", "peer-review"},
			},
		},
	}
}

func TestBuildUsesReadOnlyNonInteractiveStructuredOutput(t *testing.T) {
	workDir := t.TempDir()
	invocation, err := (Adapter{Binary: "codex"}).Build(requestFixture(), workDir)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	joined := strings.Join(invocation.Args, " ")
	for _, expected := range []string{
		"exec",
		"--skip-git-repo-check",
		"--sandbox read-only",
		`-c approval_policy="never"`,
		"--output-schema",
		"--output-last-message",
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("args %q missing %q", joined, expected)
		}
	}
	if invocation.Args[len(invocation.Args)-1] != "-" {
		t.Fatalf("last arg = %q, want stdin marker", invocation.Args[len(invocation.Args)-1])
	}
	if !strings.Contains(invocation.Stdin, "Allowed capabilities") ||
		!strings.Contains(invocation.Stdin, "frontend-development") ||
		!strings.Contains(invocation.Stdin, "Build and maintain user-facing") ||
		!strings.Contains(invocation.Stdin, "inspect existing code or design") {
		t.Fatalf("prompt = %q", invocation.Stdin)
	}
}

func TestSchemaConstrainsTaxonomy(t *testing.T) {
	data, err := classificationSchema(requestFixture())
	if err != nil {
		t.Fatalf("classificationSchema() error = %v", err)
	}

	var schema map[string]any
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatal(err)
	}
	properties := schema["properties"].(map[string]any)
	intent := properties["intent"].(map[string]any)
	capabilities := properties["capabilities"].(map[string]any)
	items := capabilities["items"].(map[string]any)

	if !reflect.DeepEqual(intent["enum"], []any{"", "implement", "review"}) {
		t.Fatalf("intent enum = %#v", intent["enum"])
	}
	if !reflect.DeepEqual(items["enum"], []any{"frontend-development", "code-review"}) {
		t.Fatalf("capability enum = %#v", items["enum"])
	}
}

func TestDecodeReadsStructuredResult(t *testing.T) {
	workDir := t.TempDir()
	data := []byte(`{
  "intent": "implement",
  "capabilities": ["frontend-development"],
  "can_classify": true,
  "clarification_question": "",
  "rationale": "UI context"
}`)
	if err := os.WriteFile(filepath.Join(workDir, resultFile), data, 0o600); err != nil {
		t.Fatal(err)
	}

	response, err := (Adapter{}).Decode(workDir)
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if response.Intent != orchestration.IntentImplement {
		t.Fatalf("intent = %q", response.Intent)
	}
	if !reflect.DeepEqual(response.Capabilities, []string{"frontend-development"}) {
		t.Fatalf("capabilities = %#v", response.Capabilities)
	}
}


func TestSchemaAvoidsUnsupportedUniqueItems(t *testing.T) {
	data, err := classificationSchema(requestFixture())
	if err != nil {
		t.Fatalf("classificationSchema() error = %v", err)
	}
	if strings.Contains(string(data), "uniqueItems") {
		t.Fatalf("schema contains unsupported uniqueItems: %s", string(data))
	}
}
