package main

import (
	"context"
	"strings"
	"testing"

	"github.com/FernandoVillegas27/ai-engineering-os/core/detector"
	"github.com/FernandoVillegas27/ai-engineering-os/core/registry"
	"github.com/FernandoVillegas27/ai-engineering-os/core/toolavailability"
	"github.com/FernandoVillegas27/ai-engineering-os/core/toolhealth"
	context7integration "github.com/FernandoVillegas27/ai-engineering-os/integrations/context7"
	githubintegration "github.com/FernandoVillegas27/ai-engineering-os/integrations/githubcli"
)

func testTool(id, interfaceType, provider, status string) registry.Tool {
	var providerValue *string
	if provider != "" {
		value := provider
		providerValue = &value
	}
	return registry.Tool{
		ID: id,
		Description: id,
		Interface: registry.ToolInterface{Type: interfaceType, Provider: providerValue},
		Operations: []string{"read"},
		Access: []string{"read"},
		Risk: "low",
		Status: status,
	}
}

func TestCollectToolAvailabilityUsesOnlyImplementedProbes(t *testing.T) {
	tools := registry.ToolRegistry{SchemaVersion: 1, Tools: map[string]registry.Tool{
		"filesystem": testTool("filesystem", "native", "local", "enabled"),
		"engram": testTool("engram", "mcp", "engram", "enabled"),
		"github": testTool("github", "connector", "github", "enabled"),
		"disabled": testTool("disabled", "mcp", "disabled", "disabled"),
	}}
	calls := 0
	snapshot := collectToolAvailability(context.Background(), tools, map[string]toolProbe{
		"engram": func(context.Context, registry.Tool) toolavailability.Evidence {
			calls++
			return toolavailability.Evidence{
				ToolID: "engram", Provider: "engram",
				State: toolavailability.StateAvailable, Healthy: true,
			}
		},
	})
	if calls != 1 {
		t.Fatalf("calls=%d", calls)
	}
	if snapshot["filesystem"].State != toolavailability.StateAvailable {
		t.Fatalf("filesystem=%#v", snapshot["filesystem"])
	}
	if snapshot["engram"].State != toolavailability.StateAvailable {
		t.Fatalf("engram=%#v", snapshot["engram"])
	}
	if snapshot["github"].State != toolavailability.StateUnknown {
		t.Fatalf("github=%#v", snapshot["github"])
	}
	if snapshot["disabled"].State != toolavailability.StateUnavailable {
		t.Fatalf("disabled=%#v", snapshot["disabled"])
	}
}

func TestContextStyleHealthyLauncherCanRemainUnknown(t *testing.T) {
	tool := testTool("context7", "mcp", "context7", "enabled")
	evidence := toolavailability.FromHealth(tool, toolhealth.Status{
		Provider: "context7", Available: true, Healthy: true,
	}, false)
	if evidence.State != toolavailability.StateUnknown || !evidence.Healthy {
		t.Fatalf("evidence=%#v", evidence)
	}
}

func TestPrintToolAvailabilityDoesNotExposeSecrets(t *testing.T) {
	snapshot := toolavailability.Snapshot{
		"engram": {
			ToolID: "engram", Provider: "engram",
			State: toolavailability.StateUnavailable,
			Details: "token=super-secret",
		},
	}
	// Verify the same redactor used by the printer removes secrets.
	if got := toolhealth.Redact(snapshot["engram"].Details); strings.Contains(got, "super-secret") {
		t.Fatalf("redaction failed: %q", got)
	}
}


func TestParseToolsStatusArgsRequiresExplicitProjectForBindingVerification(t *testing.T) {
	options, err := parseToolsStatusArgs([]string{"--project", ".", "--verify-bindings", "--json"})
	if err != nil {
		t.Fatal(err)
	}
	if options.projectRoot != "." || !options.verifyBindings || !options.asJSON {
		t.Fatalf("options=%#v", options)
	}
	if _, err := parseToolsStatusArgs([]string{"--verify-bindings"}); err == nil ||
		!strings.Contains(err.Error(), "requiere --project") {
		t.Fatalf("err=%v", err)
	}
}

func TestParseToolsStatusArgsProjectAloneDoesNotEnableExternalVerification(t *testing.T) {
	options, err := parseToolsStatusArgs([]string{"--project", "."})
	if err != nil {
		t.Fatal(err)
	}
	if options.verifyBindings {
		t.Fatal("project path alone must not enable external binding verification")
	}
}

func TestVerifyProjectToolBindingsUsesOnlyVerifiedReadOperations(t *testing.T) {
	contextProvider := "context7"
	githubProvider := "github"
	contextCheck := "context7-ready"
	githubCheck := "github-ready"
	tools := registry.ToolRegistry{SchemaVersion: 1, Tools: map[string]registry.Tool{
		"context7": {
			ID: "context7", Description: "docs",
			Interface: registry.ToolInterface{Type: "mcp", Provider: &contextProvider},
			Operations: []string{"search-docs"},
			Access: []string{"read", "network"}, Risk: "low",
			AvailabilityCheck: &contextCheck, Status: "enabled",
		},
		"github": {
			ID: "github", Description: "github",
			Interface: registry.ToolInterface{Type: "connector", Provider: &githubProvider},
			Operations: []string{
				"read-repository", "read-issues", "read-pull-requests",
				"create-commit",
			},
			Access: []string{"read", "write", "network"}, Risk: "high",
			RequiresApproval: true,
			ApprovalRequiredOperations: []string{"create-commit"},
			AvailabilityCheck: &githubCheck, Status: "enabled",
		},
	}}
	base := collectToolAvailability(context.Background(), tools, nil)
	contextCalls := 0
	githubCalls := 0
	got := verifyProjectToolBindings(
		context.Background(),
		tools,
		base,
		t.TempDir(),
		detector.Detection{
			Frontend: []detector.Technology{{Name: "angular"}},
		},
		func(context.Context, context7integration.SearchRequest) (context7integration.SearchResult, error) {
			contextCalls++
			return context7integration.SearchResult{
				Libraries: []string{"Angular"},
				Context: "docs",
			}, nil
		},
		func(context.Context, string) (githubintegration.Snapshot, error) {
			githubCalls++
			return githubintegration.Snapshot{
				BoundOperations: []string{"read-repository", "read-pull-requests"},
				Context: "github",
			}, nil
		},
	)
	if contextCalls != 1 || githubCalls != 1 {
		t.Fatalf("contextCalls=%d githubCalls=%d", contextCalls, githubCalls)
	}
	if got["context7"].RuntimeBound == false ||
		strings.Join(got["context7"].BoundOperations, ",") != "search-docs" {
		t.Fatalf("context7=%#v", got["context7"])
	}
	if strings.Join(got["github"].BoundOperations, ",") != "read-pull-requests,read-repository" {
		t.Fatalf("github=%#v", got["github"])
	}
	if err := toolavailability.ValidateSnapshot(tools, got); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyProjectToolBindingsSkipsContext7QueryWithoutHints(t *testing.T) {
	contextProvider := "context7"
	contextCheck := "context7-ready"
	tools := registry.ToolRegistry{SchemaVersion: 1, Tools: map[string]registry.Tool{
		"context7": {
			ID: "context7", Description: "docs",
			Interface: registry.ToolInterface{Type: "mcp", Provider: &contextProvider},
			Operations: []string{"search-docs"},
			Access: []string{"read", "network"}, Risk: "low",
			AvailabilityCheck: &contextCheck, Status: "enabled",
		},
	}}
	calls := 0
	got := verifyProjectToolBindings(
		context.Background(),
		tools,
		collectToolAvailability(context.Background(), tools, nil),
		t.TempDir(),
		detector.Detection{},
		func(context.Context, context7integration.SearchRequest) (context7integration.SearchResult, error) {
			calls++
			return context7integration.SearchResult{}, nil
		},
		nil,
	)
	if calls != 0 {
		t.Fatalf("Context7 search calls=%d", calls)
	}
	if got["context7"].State != toolavailability.StateUnknown || got["context7"].RuntimeBound {
		t.Fatalf("context7=%#v", got["context7"])
	}
}

func TestVerifyProjectToolBindingsDoesNotMutateBaseSnapshot(t *testing.T) {
	githubProvider := "github"
	githubCheck := "github-ready"
	tools := registry.ToolRegistry{SchemaVersion: 1, Tools: map[string]registry.Tool{
		"github": {
			ID: "github", Description: "github",
			Interface: registry.ToolInterface{Type: "connector", Provider: &githubProvider},
			Operations: []string{"read-repository"},
			Access: []string{"read", "network"}, Risk: "high",
			AvailabilityCheck: &githubCheck, Status: "enabled",
		},
	}}
	base := collectToolAvailability(context.Background(), tools, nil)
	if base["github"].State != toolavailability.StateUnknown {
		t.Fatalf("base=%#v", base)
	}
	_ = verifyProjectToolBindings(
		context.Background(),
		tools,
		base,
		t.TempDir(),
		detector.Detection{},
		nil,
		func(context.Context, string) (githubintegration.Snapshot, error) {
			return githubintegration.Snapshot{BoundOperations: []string{"read-repository"}}, nil
		},
	)
	if base["github"].State != toolavailability.StateUnknown || base["github"].RuntimeBound {
		t.Fatalf("base mutated=%#v", base["github"])
	}
}
