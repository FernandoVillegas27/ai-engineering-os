package classifier

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/FernandoVillegas27/ai-engineering-os/core/modelprovider"
	"github.com/FernandoVillegas27/ai-engineering-os/core/orchestration"
)

type fakeProvider struct {
	id       string
	response modelprovider.ClassificationResponse
	err      error
	request  modelprovider.ClassificationRequest
	called   bool
}

func (f *fakeProvider) ID() string {
	return f.id
}

func (f *fakeProvider) Classify(_ context.Context, request modelprovider.ClassificationRequest) (modelprovider.ClassificationResponse, error) {
	f.called = true
	f.request = request
	return f.response, f.err
}

func TestResolveSkipsAIForDeterministicClassification(t *testing.T) {
	provider := &fakeProvider{id: "fake"}

	resolution, err := Resolve(
		context.Background(),
		"Implementá un endpoint REST en el backend",
		testCapabilities(),
		"project",
		provider,
	)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	if provider.called {
		t.Fatal("provider should not be called for deterministic classification")
	}
	if resolution.Source != SourceDeterministic {
		t.Fatalf("source = %q", resolution.Source)
	}
}

func TestResolveUsesAIForAmbiguousClassification(t *testing.T) {
	provider := &fakeProvider{
		id: "fake",
		response: modelprovider.ClassificationResponse{
			Intent:       orchestration.IntentImplement,
			Capabilities: []string{"frontend-development"},
			CanClassify:  true,
			Rationale:    "project context indicates a UI change",
		},
	}

	resolution, err := Resolve(
		context.Background(),
		"Necesito mejorar esto",
		testCapabilities(),
		"Angular project",
		provider,
	)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	if !provider.called {
		t.Fatal("provider should be called")
	}
	if resolution.Source != SourceAI || resolution.Provider != "fake" {
		t.Fatalf("resolution = %#v", resolution)
	}
	if resolution.Result.Intent != orchestration.IntentImplement {
		t.Fatalf("intent = %q", resolution.Result.Intent)
	}
	if !reflect.DeepEqual(resolution.Result.Capabilities, []string{"frontend-development"}) {
		t.Fatalf("capabilities = %#v", resolution.Result.Capabilities)
	}
}

func TestResolveRejectsUnknownCapabilityFromAI(t *testing.T) {
	provider := &fakeProvider{
		id: "fake",
		response: modelprovider.ClassificationResponse{
			Intent:       orchestration.IntentImplement,
			Capabilities: []string{"super-backend"},
			CanClassify:  true,
		},
	}

	_, err := Resolve(
		context.Background(),
		"Necesito mejorar esto",
		testCapabilities(),
		"",
		provider,
	)
	if err == nil {
		t.Fatal("expected unknown capability error")
	}
}

func TestResolveRejectsUnknownIntentFromAI(t *testing.T) {
	provider := &fakeProvider{
		id: "fake",
		response: modelprovider.ClassificationResponse{
			Intent:       orchestration.Intent("ship"),
			Capabilities: []string{"backend-development"},
			CanClassify:  true,
		},
	}

	_, err := Resolve(
		context.Background(),
		"Necesito mejorar esto",
		testCapabilities(),
		"",
		provider,
	)
	if err == nil {
		t.Fatal("expected unsupported intent error")
	}
}

func TestResolveReturnsClarificationInsteadOfGuessing(t *testing.T) {
	provider := &fakeProvider{
		id: "fake",
		response: modelprovider.ClassificationResponse{
			CanClassify:           false,
			ClarificationQuestion: "¿Qué parte querés mejorar: frontend, backend o base de datos?",
		},
	}

	resolution, err := Resolve(
		context.Background(),
		"Necesito mejorar esto",
		testCapabilities(),
		"",
		provider,
	)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	if !resolution.ClarificationRequired {
		t.Fatal("expected clarification")
	}
	if resolution.ClarificationQuestion == "" {
		t.Fatal("expected clarification question")
	}
}

func TestResolvePropagatesProviderFailure(t *testing.T) {
	provider := &fakeProvider{
		id:  "fake",
		err: errors.New("provider unavailable"),
	}

	_, err := Resolve(
		context.Background(),
		"Necesito mejorar esto",
		testCapabilities(),
		"",
		provider,
	)
	if err == nil {
		t.Fatal("expected provider error")
	}
}

func TestResolveConstrainsProviderRequest(t *testing.T) {
	provider := &fakeProvider{
		id: "fake",
		response: modelprovider.ClassificationResponse{
			CanClassify:           false,
			ClarificationQuestion: "¿Qué querés cambiar?",
		},
	}

	_, err := Resolve(
		context.Background(),
		"Necesito mejorar esto",
		testCapabilities(),
		"project context",
		provider,
	)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	if provider.request.ProjectContext != "project context" {
		t.Fatalf("project context = %q", provider.request.ProjectContext)
	}
	if len(provider.request.AllowedIntents) != 5 {
		t.Fatalf("allowed intents = %#v", provider.request.AllowedIntents)
	}
	if len(provider.request.AllowedCapabilities) == 0 {
		t.Fatal("expected allowed capabilities")
	}
	if provider.request.AllowedCapabilities[0].ID == "" {
		t.Fatal("expected capability id")
	}
	hasDescription := false
	for _, capability := range provider.request.AllowedCapabilities {
		if capability.Description != "" {
			hasDescription = true
			break
		}
	}
	if !hasDescription {
		t.Fatal("expected capability descriptions")
	}
}


func TestResolveSynthesizesClarificationWhenProviderOmitsQuestion(t *testing.T) {
	provider := &fakeProvider{
		id: "fake",
		response: modelprovider.ClassificationResponse{
			CanClassify: false,
		},
	}

	resolution, err := Resolve(
		context.Background(),
		"Necesito mejorar esto",
		testCapabilities(),
		"",
		provider,
	)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if !resolution.ClarificationRequired {
		t.Fatal("expected clarification")
	}
	if resolution.ClarificationQuestion == "" {
		t.Fatal("expected synthesized clarification question")
	}
}
