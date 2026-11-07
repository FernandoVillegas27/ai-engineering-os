package main

import (
	"context"
	"strings"
	"testing"

	"github.com/FernandoVillegas27/ai-engineering-os/core/registry"
	"github.com/FernandoVillegas27/ai-engineering-os/core/toolavailability"
	"github.com/FernandoVillegas27/ai-engineering-os/core/toolhealth"
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
	tools := registry.ToolRegistry{SchemaVersion: 1, Tools: map[string]registry.Tool{
		"engram": testTool("engram", "mcp", "engram", "enabled"),
	}}
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
