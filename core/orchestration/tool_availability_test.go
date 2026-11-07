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
