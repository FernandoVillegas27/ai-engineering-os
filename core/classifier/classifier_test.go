package classifier

import (
	"reflect"
	"testing"

	"github.com/FernandoVillegas27/ai-engineering-os/core/orchestration"
	"github.com/FernandoVillegas27/ai-engineering-os/core/registry"
)

func testCapabilities() registry.CapabilityRegistry {
	return registry.CapabilityRegistry{Capabilities: map[string]registry.Capability{
		"backend-development": {Status: "active", Description: "Build and maintain backend application logic and services.", Aliases: []string{"backend", "server-side"}},
		"api-development":     {Status: "active", Description: "Design and implement application programming interfaces and their contracts.", Aliases: []string{"rest-api", "http-api"}},
		"frontend-development": {Status: "active", Description: "Build and maintain user-facing web application interfaces.", Aliases: []string{"frontend", "ui-development"}},
		"database-design":     {Status: "active", Description: "Design schemas, persistence models, queries and data access patterns.", Aliases: []string{"persistence", "data-modeling"}},
		"code-review":         {Status: "active", Description: "Review code for correctness, maintainability, consistency and regressions.", Aliases: []string{"review", "peer-review"}},
		"automated-testing":   {Status: "active", Description: "Design and implement automated tests that verify requested behavior.", Aliases: []string{"testing", "test-automation"}},
		"security-review":     {Status: "active", Description: "Identify security risks and verify controls in code and system design.", Aliases: []string{"application-security", "appsec"}},
		"technical-research":  {Status: "active", Description: "Investigate technologies, APIs, documentation and implementation options.", Aliases: []string{"research", "investigation"}},
	}}
}

func TestClassifyBackendEndpointImplementation(t *testing.T) {
	got, err := Classify("Implementá un endpoint REST para productos en el backend", testCapabilities())
	if err != nil {
		t.Fatalf("Classify() error = %v", err)
	}

	if got.Intent != orchestration.IntentImplement {
		t.Fatalf("intent = %q", got.Intent)
	}
	want := []string{"api-development", "backend-development"}
	if !reflect.DeepEqual(got.Capabilities, want) {
		t.Fatalf("capabilities = %#v, want %#v", got.Capabilities, want)
	}
	if got.NeedsAI {
		t.Fatal("expected deterministic classification")
	}
}

func TestClassifyDatabaseReview(t *testing.T) {
	got, err := Classify("Revisá este cambio de base de datos", testCapabilities())
	if err != nil {
		t.Fatalf("Classify() error = %v", err)
	}

	if got.Intent != orchestration.IntentReview {
		t.Fatalf("intent = %q", got.Intent)
	}
	want := []string{"code-review", "database-design"}
	if !reflect.DeepEqual(got.Capabilities, want) {
		t.Fatalf("capabilities = %#v, want %#v", got.Capabilities, want)
	}
}

func TestClassifyFrontendImplementation(t *testing.T) {
	got, err := Classify("Agregá una pantalla Angular para editar clientes", testCapabilities())
	if err != nil {
		t.Fatalf("Classify() error = %v", err)
	}
	if got.Intent != orchestration.IntentImplement {
		t.Fatalf("intent = %q", got.Intent)
	}
	if !reflect.DeepEqual(got.Capabilities, []string{"frontend-development"}) {
		t.Fatalf("capabilities = %#v", got.Capabilities)
	}
}

func TestClassifyAmbiguousTaskNeedsAI(t *testing.T) {
	got, err := Classify("Necesito mejorar esto", testCapabilities())
	if err != nil {
		t.Fatalf("Classify() error = %v", err)
	}
	if !got.NeedsAI || got.Confidence != ConfidenceAmbiguous {
		t.Fatalf("result = %#v", got)
	}
}

func TestClassifyResearch(t *testing.T) {
	got, err := Classify("Investigá la documentación para comparar opciones de API", testCapabilities())
	if err != nil {
		t.Fatalf("Classify() error = %v", err)
	}
	if got.Intent != orchestration.IntentResearch {
		t.Fatalf("intent = %q", got.Intent)
	}
	want := []string{"api-development", "technical-research"}
	if !reflect.DeepEqual(got.Capabilities, want) {
		t.Fatalf("capabilities = %#v, want %#v", got.Capabilities, want)
	}
}


func TestReviewIntentDoesNotInventMissingCapability(t *testing.T) {
	capabilities := testCapabilities()
	delete(capabilities.Capabilities, "code-review")

	got, err := Classify("Revisá este cambio de base de datos", capabilities)
	if err != nil {
		t.Fatalf("Classify() error = %v", err)
	}

	if !reflect.DeepEqual(got.Capabilities, []string{"database-design"}) {
		t.Fatalf("capabilities = %#v", got.Capabilities)
	}
}


func TestClassifySpanishPermissionReviewWithoutAI(t *testing.T) {
	got, err := Classify(
		"Quiero que revises si la forma en que manejamos los permisos puede tener problemas",
		testCapabilities(),
	)
	if err != nil {
		t.Fatalf("Classify() error = %v", err)
	}

	if got.Intent != orchestration.IntentReview {
		t.Fatalf("intent = %q", got.Intent)
	}
	want := []string{"code-review", "security-review"}
	if !reflect.DeepEqual(got.Capabilities, want) {
		t.Fatalf("capabilities = %#v, want %#v", got.Capabilities, want)
	}
	if got.NeedsAI {
		t.Fatal("expected deterministic classification without AI")
	}
}


func TestClassifyRuntimeImplementationAsBackendDevelopment(t *testing.T) {
	registry := registry.CapabilityRegistry{
		Capabilities: map[string]registry.Capability{
			"backend-development": {
				Description: "Build and maintain backend application logic and services.",
				Aliases:     []string{"backend", "server-side"},
				Status:      "active",
			},
		},
	}

	result, err := Classify(
		"Agregá una validación para que el timeout del runtime Codex no pueda configurarse en menos de 5 segundos",
		registry,
	)
	if err != nil {
		t.Fatalf("Classify() error = %v", err)
	}
	if result.Intent != orchestration.IntentImplement {
		t.Fatalf("intent = %q, want implement", result.Intent)
	}
	if result.NeedsAI {
		t.Fatal("expected deterministic classification")
	}
	if result.Confidence != ConfidenceHigh {
		t.Fatalf("confidence = %q, want high", result.Confidence)
	}
	if !contains(result.Capabilities, "backend-development") {
		t.Fatalf("capabilities = %#v, want backend-development", result.Capabilities)
	}
}
