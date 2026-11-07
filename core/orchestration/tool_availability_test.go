package orchestration

import (
	"strings"
	"testing"

	"github.com/FernandoVillegas27/ai-engineering-os/core/registry"
	"github.com/FernandoVillegas27/ai-engineering-os/core/toolavailability"
	"github.com/FernandoVillegas27/ai-engineering-os/core/toolhealth"
)

func availabilityTool(id, interfaceType, provider string) registry.Tool {
	var providerValue *string
	if provider != "" {
		value := provider
		providerValue = &value
	}
	return registry.Tool{
		ID: id, Description: id,
		Interface: registry.ToolInterface{Type: interfaceType, Provider: providerValue},
		Operations: []string{"read"}, Access: []string{"read"},
		Risk: "low", Status: "enabled",
	}
}

func TestApplyToolAvailabilityNeverAddsTools(t *testing.T) {
	tools := registry.ToolRegistry{SchemaVersion: 1, Tools: map[string]registry.Tool{
		"filesystem": availabilityTool("filesystem", "native", ""),
		"engram": availabilityTool("engram", "mcp", "engram"),
	}}
	plan := Plan{Agents: []AgentPlan{{
		AgentID: "researcher",
		Tools: ToolPlan{Required: []string{"filesystem"}},
	}}}
	snapshot := toolavailability.Snapshot{
		"filesystem": toolavailability.Native(tools.Tools["filesystem"]),
		"engram": toolavailability.BindOperations(toolavailability.FromHealth(
			tools.Tools["engram"],
			toolhealth.Status{Provider: "engram", Available: true, Healthy: true},
			true,
		), "read"),
	}
	got, err := ApplyToolAvailability(plan, tools, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Agents[0].Tools.Recommended) != 0 {
		t.Fatalf("availability added a tool: %#v", got.Agents[0].Tools)
	}
}

func TestApplyToolAvailabilityOmitsUnboundOptionalExternalTools(t *testing.T) {
	tools := registry.ToolRegistry{SchemaVersion: 1, Tools: map[string]registry.Tool{
		"filesystem": availabilityTool("filesystem", "native", ""),
		"engram": availabilityTool("engram", "mcp", "engram"),
	}}
	plan := Plan{Agents: []AgentPlan{{
		AgentID: "researcher",
		Tools: ToolPlan{
			Required: []string{"filesystem"},
			Recommended: []string{"engram"},
		},
	}}}
	snapshot := toolavailability.Snapshot{
		"filesystem": toolavailability.Native(tools.Tools["filesystem"]),
		"engram": toolavailability.FromHealth(
			tools.Tools["engram"],
			toolhealth.Status{Provider: "engram", Available: true, Healthy: true},
			true,
		),
	}
	got, err := ApplyToolAvailability(plan, tools, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Agents[0].Tools.Recommended) != 0 {
		t.Fatalf("recommended=%#v", got.Agents[0].Tools.Recommended)
	}
}

func TestApplyToolAvailabilityBlocksRequiredUnboundExternalTool(t *testing.T) {
	tools := registry.ToolRegistry{SchemaVersion: 1, Tools: map[string]registry.Tool{
		"engram": availabilityTool("engram", "mcp", "engram"),
	}}
	plan := Plan{Agents: []AgentPlan{{
		AgentID: "researcher",
		Tools: ToolPlan{Required: []string{"engram"}},
	}}}
	snapshot := toolavailability.Snapshot{
		"engram": toolavailability.FromHealth(
			tools.Tools["engram"],
			toolhealth.Status{Provider: "engram", Available: true, Healthy: true},
			true,
		),
	}
	_, err := ApplyToolAvailability(plan, tools, snapshot)
	if err == nil || !strings.Contains(err.Error(), "not bound to the selected execution runtime") {
		t.Fatalf("err=%v", err)
	}
}


func TestApplyToolAvailabilityCarriesEffectiveOperations(t *testing.T) {
	provider := "github"
	tools := registry.ToolRegistry{SchemaVersion: 1, Tools: map[string]registry.Tool{
		"filesystem": {
			ID: "filesystem", Description: "fs",
			Interface: registry.ToolInterface{Type: "native"},
			Operations: []string{"read-files", "write-files"},
			Access: []string{"read", "write"}, Risk: "low", Status: "enabled",
		},
		"github": {
			ID: "github", Description: "gh",
			Interface: registry.ToolInterface{Type: "connector", Provider: &provider},
			Operations: []string{"read-repository", "create-pull-request"},
			Access: []string{"read", "write", "network"}, Risk: "high", Status: "enabled",
		},
	}}
	plan := Plan{Agents: []AgentPlan{{
		AgentID: "researcher",
		Tools: ToolPlan{Required: []string{"filesystem"}, Recommended: []string{"github"}},
	}}}
	snapshot := toolavailability.Snapshot{
		"filesystem": toolavailability.Native(tools.Tools["filesystem"]),
		"github": toolavailability.BindOperations(
			toolavailability.FromHealth(
				tools.Tools["github"],
				toolhealth.Status{Provider: "github", Available: true, Healthy: true},
				true,
			),
			"read-repository",
		),
	}
	got, err := ApplyToolAvailability(plan, tools, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	ops := got.Agents[0].Tools.Operations
	if strings.Join(ops["filesystem"], ",") != "read-files,write-files" {
		t.Fatalf("filesystem ops=%#v", ops["filesystem"])
	}
	if strings.Join(ops["github"], ",") != "read-repository" {
		t.Fatalf("github ops=%#v", ops["github"])
	}
}
