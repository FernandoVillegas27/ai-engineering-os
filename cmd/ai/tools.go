package main

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/FernandoVillegas27/ai-engineering-os/core/registry"
	"github.com/FernandoVillegas27/ai-engineering-os/core/toolavailability"
	"github.com/FernandoVillegas27/ai-engineering-os/core/toolhealth"
	context7integration "github.com/FernandoVillegas27/ai-engineering-os/integrations/context7"
	engramintegration "github.com/FernandoVillegas27/ai-engineering-os/integrations/engram"
)

type toolProbe func(context.Context, registry.Tool) toolavailability.Evidence

func runTools(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("uso: ai tools status [--json]")
	}
	switch args[0] {
	case "status":
		return runToolsStatus(args[1:])
	default:
		return fmt.Errorf("subcomando tools desconocido: %s", args[0])
	}
}

func runToolsStatus(args []string) error {
	asJSON := false
	for _, arg := range args {
		switch arg {
		case "--json":
			asJSON = true
		case "-h", "--help":
			return fmt.Errorf("uso: ai tools status [--json]")
		default:
			return fmt.Errorf("opción desconocida: %s", arg)
		}
	}

	path, err := resolveRegistryFile("tools.yaml")
	if err != nil {
		return err
	}
	tools, err := registry.LoadTools(path)
	if err != nil {
		return err
	}

	snapshot := collectToolAvailability(
		context.Background(),
		tools,
		defaultToolProbes(),
	)
	if err := toolavailability.ValidateSnapshot(tools, snapshot); err != nil {
		return err
	}

	if asJSON {
		return printJSON(snapshot)
	}
	printToolAvailability(tools, snapshot)
	return nil
}

func defaultToolProbes() map[string]toolProbe {
	return map[string]toolProbe{
		"engram": func(ctx context.Context, tool registry.Tool) toolavailability.Evidence {
			health := engramintegration.New().Health(ctx)
			return toolavailability.FromHealth(tool, health, true)
		},
		"context7": func(ctx context.Context, tool registry.Tool) toolavailability.Evidence {
			health := context7integration.New().Health(ctx)
			if !health.Available || !health.Healthy {
				return toolavailability.FromHealth(tool, health, false)
			}
			return toolavailability.Unknown(
				tool,
				"local MCP launcher is healthy; remote MCP host connectivity is not verified",
			)
		},
	}
}

func collectToolAvailability(
	ctx context.Context,
	tools registry.ToolRegistry,
	probes map[string]toolProbe,
) toolavailability.Snapshot {
	snapshot := toolavailability.Snapshot{}
	ids := make([]string, 0, len(tools.Tools))
	for id := range tools.Tools {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	for _, id := range ids {
		tool := tools.Tools[id]
		if tool.Status != "enabled" {
			snapshot[id] = toolavailability.Evidence{
				ToolID: id,
				State: toolavailability.StateUnavailable,
				Healthy: false,
				Details: "tool is disabled in registry",
			}
			continue
		}
		if tool.Interface.Type == "native" {
			snapshot[id] = toolavailability.Native(tool)
			continue
		}
		if probe, ok := probes[id]; ok {
			snapshot[id] = probe(ctx, tool)
			continue
		}
		snapshot[id] = toolavailability.Unknown(
			tool,
			"no local runtime availability adapter is implemented",
		)
	}
	return snapshot
}

func printToolAvailability(tools registry.ToolRegistry, snapshot toolavailability.Snapshot) {
	fmt.Println("Tool Availability")
	fmt.Println()
	ids := make([]string, 0, len(snapshot))
	for id := range snapshot {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	for _, id := range ids {
		tool := tools.Tools[id]
		evidence := snapshot[id]
		provider := strings.TrimSpace(evidence.Provider)
		if provider == "" {
			provider = "-"
		}
		health := "unknown"
		if evidence.State == toolavailability.StateAvailable {
			health = "healthy"
		} else if evidence.State == toolavailability.StateUnavailable {
			health = "unavailable"
		}
		fmt.Printf(
			"%-12s %-10s %-12s %-11s %s\n",
			id,
			tool.Interface.Type,
			provider,
			evidence.State,
			health,
		)
		if detail := strings.TrimSpace(evidence.Details); detail != "" {
			fmt.Printf("  %s\n", toolhealth.Redact(detail))
		}
	}
	fmt.Println()
	fmt.Println("Availability does not grant permissions. External unknown means not verified, not implicitly available.")
}
