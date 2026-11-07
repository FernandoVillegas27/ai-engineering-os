package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/FernandoVillegas27/ai-engineering-os/core/detector"
	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/orchestration"
	"github.com/FernandoVillegas27/ai-engineering-os/core/registry"
	"github.com/FernandoVillegas27/ai-engineering-os/core/toolavailability"
	context7integration "github.com/FernandoVillegas27/ai-engineering-os/integrations/context7"
)

func context7TestRegistry() registry.ToolRegistry {
	provider := "context7"
	check := "context7-available"
	return registry.ToolRegistry{
		SchemaVersion: 1,
		Tools: map[string]registry.Tool{
			"filesystem": {
				ID: "filesystem", Description: "fs", Status: "enabled",
				Interface: registry.ToolInterface{Type: "native"},
				Operations: []string{"read"}, Access: []string{"read"}, Risk: "low",
			},
			"context7": {
				ID: "context7", Description: "docs", Status: "enabled",
				Interface: registry.ToolInterface{Type: "mcp", Provider: &provider},
				Operations: []string{"resolve-library-id", "query-docs", "search-docs"},
				Access: []string{"read", "network"}, Risk: "low",
				AvailabilityCheck: &check,
			},
		},
	}
}

func context7TestPlan() orchestration.Plan {
	return orchestration.Plan{
		Intent: orchestration.IntentResearch,
		Agents: []orchestration.AgentPlan{{
			AgentID: "researcher",
			Tools: orchestration.ToolPlan{
				Recommended: []string{"context7"},
			},
		}},
	}
}

func TestContext7BindingUsesOnlyDetectedPublicLibraryHints(t *testing.T) {
	var request context7integration.SearchRequest
	bindings := collectRuntimeToolBindingsWithSearch(
		context.Background(),
		context7TestRegistry(),
		context7TestPlan(),
		detector.Detection{
			Frontend: []detector.Technology{
				{Name: "angular", Version: "21"},
				{Name: "typescript", Version: "5"},
			},
			Backend: []detector.Technology{{Name: "spring-boot", Version: "3"}},
		},
		func(_ context.Context, got context7integration.SearchRequest) (context7integration.SearchResult, error) {
			request = got
			return context7integration.SearchResult{
				Context: "Context7 external documentation evidence\n{}",
				Libraries: got.Libraries,
			}, nil
		},
	)

	evidence := bindings.Availability["context7"]
	if evidence.State != toolavailability.StateAvailable || !evidence.RuntimeBound || !evidence.Healthy {
		t.Fatalf("evidence=%#v", evidence)
	}
	if strings.Join(evidence.BoundOperations, ",") != "search-docs" {
		t.Fatalf("bound operations=%#v", evidence.BoundOperations)
	}
	if request.Query != context7DocumentationQuery {
		t.Fatalf("query=%q", request.Query)
	}
	if strings.Contains(strings.ToLower(request.Query), "customer") ||
		strings.Contains(strings.ToLower(request.Query), "task") {
		t.Fatalf("query unexpectedly includes task/private context: %q", request.Query)
	}
	if got := strings.Join(request.Libraries, ","); got != "Angular,Spring Boot,TypeScript" {
		t.Fatalf("libraries=%q", got)
	}
	if request.Language != "TypeScript" {
		t.Fatalf("language=%q", request.Language)
	}
	if bindings.Context["context7"] == "" {
		t.Fatal("expected Context7 evidence context")
	}
}

func TestContext7BindingSkipsNetworkWithoutPublicLibraryHints(t *testing.T) {
	called := false
	bindings := collectRuntimeToolBindingsWithSearch(
		context.Background(),
		context7TestRegistry(),
		context7TestPlan(),
		detector.Detection{},
		func(context.Context, context7integration.SearchRequest) (context7integration.SearchResult, error) {
			called = true
			return context7integration.SearchResult{}, nil
		},
	)
	if called {
		t.Fatal("external search must not run without public library hints")
	}
	if got := bindings.Availability["context7"]; got.State != toolavailability.StateUnknown || got.RuntimeBound {
		t.Fatalf("evidence=%#v", got)
	}
}

func TestContext7BindingFailsClosedAndOptionalToolIsRemoved(t *testing.T) {
	tools := context7TestRegistry()
	bindings := collectRuntimeToolBindingsWithSearch(
		context.Background(),
		tools,
		context7TestPlan(),
		detector.Detection{Frontend: []detector.Technology{{Name: "angular"}}},
		func(context.Context, context7integration.SearchRequest) (context7integration.SearchResult, error) {
			return context7integration.SearchResult{}, errors.New("429 rate limit")
		},
	)
	evidence := bindings.Availability["context7"]
	if evidence.State != toolavailability.StateUnavailable || evidence.RuntimeBound {
		t.Fatalf("evidence=%#v", evidence)
	}

	plan, err := orchestration.ApplyToolAvailability(context7TestPlan(), tools, bindings.Availability)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Agents) != 1 || len(plan.Agents[0].Tools.Recommended) != 0 {
		t.Fatalf("plan=%#v", plan)
	}
}

func TestInjectRuntimeToolContextTargetsOnlyDeclaredToolUsers(t *testing.T) {
	units := []execution.WorkUnit{
		{ID: "with", Tools: []string{"context7"}, CollaborationContext: "memory"},
		{ID: "without", Tools: []string{"filesystem"}, CollaborationContext: "memory"},
	}
	got := injectRuntimeToolContext(units, map[string]string{
		"context7": "external docs",
	})
	if got[0].CollaborationContext != "memory\n\nexternal docs" {
		t.Fatalf("with context=%q", got[0].CollaborationContext)
	}
	if got[1].CollaborationContext != "memory" {
		t.Fatalf("without context=%q", got[1].CollaborationContext)
	}
	if units[0].CollaborationContext != "memory" {
		t.Fatal("input work unit was mutated")
	}
}

func TestContext7LibraryHintsAreBoundedAndDeterministic(t *testing.T) {
	libraries, language := context7LibraryHints(detector.Detection{
		Frontend: []detector.Technology{
			{Name: "react"}, {Name: "angular"}, {Name: "nextjs"}, {Name: "typescript"},
		},
		Backend: []detector.Technology{
			{Name: "spring-boot"}, {Name: "java"}, {Name: "python"},
		},
	})
	if len(libraries) != 4 {
		t.Fatalf("libraries=%#v", libraries)
	}
	if got := strings.Join(libraries, ","); got != "Angular,Next.js,React,Spring Boot" {
		t.Fatalf("libraries=%q", got)
	}
	if language != "TypeScript" {
		t.Fatalf("language=%q", language)
	}
}
