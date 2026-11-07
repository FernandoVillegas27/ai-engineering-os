package main

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/FernandoVillegas27/ai-engineering-os/core/detector"
	"github.com/FernandoVillegas27/ai-engineering-os/core/registry"
	"github.com/FernandoVillegas27/ai-engineering-os/core/toolavailability"
	"github.com/FernandoVillegas27/ai-engineering-os/core/toolhealth"
	context7integration "github.com/FernandoVillegas27/ai-engineering-os/integrations/context7"
	engramintegration "github.com/FernandoVillegas27/ai-engineering-os/integrations/engram"
	githubintegration "github.com/FernandoVillegas27/ai-engineering-os/integrations/githubcli"
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

type toolsStatusOptions struct {
	asJSON         bool
	projectRoot    string
	verifyBindings bool
}

func parseToolsStatusArgs(args []string) (toolsStatusOptions, error) {
	var options toolsStatusOptions
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			options.asJSON = true
		case "--project":
			if i+1 >= len(args) {
				return toolsStatusOptions{}, fmt.Errorf("--project requiere una ruta")
			}
			i++
			options.projectRoot = args[i]
		case "--verify-bindings":
			options.verifyBindings = true
		case "-h", "--help":
			return toolsStatusOptions{}, fmt.Errorf("uso: ai tools status [--json] [--project ruta --verify-bindings]")
		default:
			return toolsStatusOptions{}, fmt.Errorf("opción desconocida: %s", args[i])
		}
	}
	if options.verifyBindings && strings.TrimSpace(options.projectRoot) == "" {
		return toolsStatusOptions{}, fmt.Errorf("--verify-bindings requiere --project")
	}
	return options, nil
}

func runToolsStatus(args []string) error {
	options, err := parseToolsStatusArgs(args)
	if err != nil {
		return err
	}

	path, err := resolveRegistryFile("tools.yaml")
	if err != nil {
		return err
	}
	tools, err := registry.LoadTools(path)
	if err != nil {
		return err
	}

	ctx := context.Background()
	snapshot := collectToolAvailability(ctx, tools, defaultToolProbes())
	if strings.TrimSpace(options.projectRoot) != "" && options.verifyBindings {
		absoluteRoot, err := filepath.Abs(options.projectRoot)
		if err != nil {
			return fmt.Errorf("resolver ruta del proyecto: %w", err)
		}
		detection, err := detector.Inspect(absoluteRoot)
		if err != nil {
			return fmt.Errorf("inspeccionar proyecto para tool bindings: %w", err)
		}
		snapshot = verifyProjectToolBindings(
			ctx,
			tools,
			snapshot,
			absoluteRoot,
			detection,
			context7integration.New().Search,
			githubintegration.New().Snapshot,
		)
	}
	if err := toolavailability.ValidateSnapshot(tools, snapshot); err != nil {
		return err
	}

	if options.asJSON {
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
		if evidence.Healthy {
			health = "healthy"
		} else if evidence.State == toolavailability.StateUnavailable {
			health = "unhealthy"
		}
		binding := "no"
		if evidence.RuntimeBound {
			binding = "yes"
		}
		fmt.Printf(
			"%-12s %-10s %-12s %-11s health=%-11s bound=%s\n",
			id,
			tool.Interface.Type,
			provider,
			evidence.State,
			health,
			binding,
		)
		if len(evidence.BoundOperations) > 0 {
			fmt.Printf("  operations: %s\n", strings.Join(evidence.BoundOperations, ", "))
		}
		if detail := strings.TrimSpace(evidence.Details); detail != "" {
			fmt.Printf("  %s\n", toolhealth.Redact(detail))
		}
	}
	fmt.Println()
	fmt.Println("Availability does not grant permissions. External unknown means not verified, not implicitly available.")
}


func verifyProjectToolBindings(
	ctx context.Context,
	tools registry.ToolRegistry,
	base toolavailability.Snapshot,
	root string,
	detection detector.Detection,
	context7Search context7SearchFunc,
	githubSnapshot githubSnapshotFunc,
) toolavailability.Snapshot {
	snapshot := make(toolavailability.Snapshot, len(base))
	for id, evidence := range base {
		snapshot[id] = evidence
	}

	if tool, ok := tools.Tools["context7"]; ok && tool.Status == "enabled" {
		libraries, language := context7LibraryHints(detection)
		switch {
		case len(libraries) == 0:
			snapshot["context7"] = toolavailability.Unknown(
				tool,
				"project binding verification found no public library hints; no Context7 external query was sent",
			)
		case context7Search == nil:
			snapshot["context7"] = toolavailability.Unknown(
				tool,
				"Context7 project binding verifier is not configured",
			)
		default:
			result, err := context7Search(ctx, context7integration.SearchRequest{
				Query: context7DocumentationQuery,
				Libraries: libraries,
				Language: language,
			})
			if err != nil {
				failure := toolhealth.Classify(err, "")
				snapshot["context7"] = toolavailability.FromHealth(
					tool,
					toolhealth.Status{
						Provider: context7integration.ProviderID,
						Available: failure.Kind != toolhealth.FailureUnavailable,
						Healthy: false,
						Details: err.Error(),
						Failure: failure,
					},
					false,
				)
			} else {
				evidence := toolavailability.FromHealth(
					tool,
					toolhealth.Status{
						Provider: context7integration.ProviderID,
						Available: true,
						Healthy: true,
						Details: fmt.Sprintf(
							"project-scoped Context7 binding verified for %d public library hint(s)",
							len(result.Libraries),
						),
					},
					true,
				)
				snapshot["context7"] = toolavailability.BindOperations(evidence, "search-docs")
			}
		}
	}

	if tool, ok := tools.Tools["github"]; ok && tool.Status == "enabled" {
		if githubSnapshot == nil {
			snapshot["github"] = toolavailability.Unknown(
				tool,
				"GitHub project binding verifier is not configured",
			)
		} else {
			result, err := githubSnapshot(ctx, root)
			if err != nil {
				failure := toolhealth.Classify(err, "")
				snapshot["github"] = toolavailability.FromHealth(
					tool,
					toolhealth.Status{
						Provider: githubintegration.ProviderID,
						Available: failure.Kind != toolhealth.FailureUnavailable,
						Healthy: false,
						Details: err.Error(),
						Failure: failure,
					},
					false,
				)
			} else {
				evidence := toolavailability.FromHealth(
					tool,
					toolhealth.Status{
						Provider: githubintegration.ProviderID,
						Available: true,
						Healthy: true,
						Details: fmt.Sprintf(
							"project-scoped GitHub read binding verified %d operation(s)",
							len(result.BoundOperations),
						),
					},
					true,
				)
				snapshot["github"] = toolavailability.BindOperations(
					evidence,
					result.BoundOperations...,
				)
			}
		}
	}

	return snapshot
}
