package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/FernandoVillegas27/ai-engineering-os/core/coordinator"
	"github.com/FernandoVillegas27/ai-engineering-os/core/detector"
	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/orchestration"
	"github.com/FernandoVillegas27/ai-engineering-os/core/registry"
	"github.com/FernandoVillegas27/ai-engineering-os/core/toolavailability"
	"github.com/FernandoVillegas27/ai-engineering-os/core/toolhealth"
	context7integration "github.com/FernandoVillegas27/ai-engineering-os/integrations/context7"
	githubintegration "github.com/FernandoVillegas27/ai-engineering-os/integrations/githubcli"
)

const context7DocumentationQuery = "Current API usage, compatibility notes, and recommended patterns for the detected public project libraries."

type runtimeToolBindings struct {
	Availability toolavailability.Snapshot
	Context      map[string]string
}

type context7SearchFunc func(context.Context, context7integration.SearchRequest) (context7integration.SearchResult, error)
type githubSnapshotFunc func(context.Context, string) (githubintegration.Snapshot, error)

func collectRuntimeToolBindings(
	ctx context.Context,
	tools registry.ToolRegistry,
	plan orchestration.Plan,
	detection detector.Detection,
	root string,
) runtimeToolBindings {
	context7Client := context7integration.New()
	bindings := collectRuntimeToolBindingsWithSearch(ctx, tools, plan, detection, context7Client.Search)
	githubClient := githubintegration.New()
	return bindGitHubReadOnly(ctx, tools, plan, root, bindings, githubClient.Snapshot)
}

func collectRuntimeToolBindingsWithSearch(
	ctx context.Context,
	tools registry.ToolRegistry,
	plan orchestration.Plan,
	detection detector.Detection,
	search context7SearchFunc,
) runtimeToolBindings {
	bindings := runtimeToolBindings{
		Availability: collectToolAvailability(ctx, tools, nil),
		Context:      map[string]string{},
	}

	tool, exists := tools.Tools["context7"]
	if !exists || tool.Status != "enabled" || !planUsesTool(plan, "context7") {
		return bindings
	}

	libraries, language := context7LibraryHints(detection)
	if len(libraries) == 0 {
		bindings.Availability["context7"] = toolavailability.Unknown(
			tool,
			"Context7 was declared by the plan but no public library hints were detected; no external query was sent",
		)
		return bindings
	}

	if search == nil {
		bindings.Availability["context7"] = toolavailability.Unknown(
			tool,
			"Context7 binding search adapter is not configured",
		)
		return bindings
	}

	result, err := search(ctx, context7integration.SearchRequest{
		Query:     context7DocumentationQuery,
		Libraries: libraries,
		Language:  language,
	})
	if err != nil {
		failure := toolhealth.Classify(err, "")
		bindings.Availability["context7"] = toolavailability.FromHealth(
			tool,
			toolhealth.Status{
				Provider:  context7integration.ProviderID,
				Available: true,
				Healthy:   false,
				Details:   err.Error(),
				Failure:   failure,
			},
			false,
		)
		return bindings
	}

	evidence := toolavailability.FromHealth(
		tool,
		toolhealth.Status{
			Provider:  context7integration.ProviderID,
			Available: true,
			Healthy:   true,
			Details: fmt.Sprintf(
				"AI Engineering OS-owned Context7 documentation prefetch succeeded for %d public library hint(s)",
				len(result.Libraries),
			),
		},
		true,
	)
	bindings.Availability["context7"] = toolavailability.BindOperations(evidence, "search-docs")
	bindings.Context["context7"] = result.Context
	return bindings
}

func bindGitHubReadOnly(
	ctx context.Context,
	tools registry.ToolRegistry,
	plan orchestration.Plan,
	root string,
	bindings runtimeToolBindings,
	snapshot githubSnapshotFunc,
) runtimeToolBindings {
	tool, exists := tools.Tools["github"]
	if !exists || tool.Status != "enabled" || !planUsesTool(plan, "github") {
		return bindings
	}
	if strings.TrimSpace(root) == "" || snapshot == nil {
		bindings.Availability["github"] = toolavailability.Unknown(
			tool,
			"GitHub read-only binding requires a project root and adapter",
		)
		return bindings
	}

	result, err := snapshot(ctx, root)
	if err != nil {
		failure := toolhealth.Classify(err, "")
		available := failure.Kind != toolhealth.FailureUnavailable
		bindings.Availability["github"] = toolavailability.FromHealth(
			tool,
			toolhealth.Status{
				Provider: githubintegration.ProviderID,
				Available: available,
				Healthy: false,
				Details: err.Error(),
				Failure: failure,
			},
			false,
		)
		return bindings
	}

	details := fmt.Sprintf(
		"AI Engineering OS-owned GitHub read binding verified %d operation(s)",
		len(result.BoundOperations),
	)
	if len(result.Warnings) > 0 {
		details += fmt.Sprintf("; %d read operation(s) unavailable", len(result.Warnings))
	}
	evidence := toolavailability.FromHealth(
		tool,
		toolhealth.Status{
			Provider: githubintegration.ProviderID,
			Available: true,
			Healthy: true,
			Details: details,
		},
		true,
	)
	bindings.Availability["github"] = toolavailability.BindOperations(
		evidence,
		result.BoundOperations...,
	)
	if strings.TrimSpace(result.Context) != "" {
		bindings.Context["github"] = result.Context
	}
	return bindings
}

func injectRuntimeToolContext(
	units []execution.WorkUnit,
	contextByTool map[string]string,
) []execution.WorkUnit {
	out := append([]execution.WorkUnit(nil), units...)
	for i := range out {
		for _, toolID := range out[i].Tools {
			contextValue := strings.TrimSpace(contextByTool[toolID])
			if contextValue == "" {
				continue
			}
			out[i].CollaborationContext = coordinator.MergeContext(
				out[i].CollaborationContext,
				contextValue,
				coordinator.DefaultCombinedContextMax,
			)
		}
	}
	return out
}

func planUsesTool(plan orchestration.Plan, toolID string) bool {
	for _, agent := range plan.Agents {
		for _, id := range append(
			append([]string(nil), agent.Tools.Required...),
			agent.Tools.Recommended...,
		) {
			if id == toolID {
				return true
			}
		}
	}
	return false
}

func context7LibraryHints(detection detector.Detection) ([]string, string) {
	priority := []struct {
		name     string
		library  string
		language string
	}{
		{"angular", "Angular", "TypeScript"},
		{"nextjs", "Next.js", "TypeScript"},
		{"react", "React", "TypeScript"},
		{"spring-boot", "Spring Boot", "Java"},
		{"django", "Django", "Python"},
		{"pytest", "pytest", "Python"},
		{"typescript", "TypeScript", "TypeScript"},
		{"java", "Java", "Java"},
		{"go", "Go", "Go"},
		{"python", "Python", "Python"},
	}

	detected := map[string]bool{}
	for _, group := range [][]detector.Technology{
		detection.Frontend,
		detection.Backend,
		detection.Testing,
		detection.Build,
		detection.Runtime,
		detection.Infrastructure,
	} {
		for _, technology := range group {
			detected[strings.ToLower(strings.TrimSpace(technology.Name))] = true
		}
	}

	libraries := make([]string, 0, 4)
	language := ""
	for _, candidate := range priority {
		if !detected[candidate.name] {
			continue
		}
		if len(libraries) < 4 {
			libraries = append(libraries, candidate.library)
		}
		if language == "" && candidate.language != "" {
			language = candidate.language
		}
	}
	return libraries, language
}
