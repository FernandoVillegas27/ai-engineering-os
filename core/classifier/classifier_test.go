package classifier

import (
	"reflect"
	"testing"

	"github.com/FernandoVillegas27/ai-engineering-os/core/orchestration"
	"github.com/FernandoVillegas27/ai-engineering-os/core/registry"
)

func testCapabilities() registry.CapabilityRegistry {
	return registry.CapabilityRegistry{Capabilities: map[string]registry.Capability{
		"backend-development": {Status: "active", Aliases: []string{"backend", "server-side"}},
		"api-development":     {Status: "active", Aliases: []string{"rest-api", "http-api"}},
		"frontend-development": {Status: "active", Aliases: []string{"frontend", "ui-development"}},
		"database-design":     {Status: "active", Aliases: []string{"persistence", "data-modeling"}},
		"code-review":         {Status: "active", Aliases: []string{"review", "peer-review"}},
		"automated-testing":   {Status: "active", Aliases: []string{"testing", "test-automation"}},
		"security-review":     {Status: "active", Aliases: []string{"application-security", "appsec"}},
		"technical-research":  {Status: "active", Aliases: []string{"research", "investigation"}},
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
