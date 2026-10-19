package orchestration

import (
	"fmt"
	"sort"

	"github.com/FernandoVillegas27/ai-engineering-os/core/detector"
	"github.com/FernandoVillegas27/ai-engineering-os/core/registry"
	"github.com/FernandoVillegas27/ai-engineering-os/core/resolver"
)

type Request struct {
	Capabilities []string
}

type AgentPlan struct {
	AgentID      string
	Capabilities []string
	Skills       SkillPlan
	Tools        ToolPlan
}

type SkillPlan struct {
	Enabled    []string
	Candidates []string
}

type ToolPlan struct {
	Required    []string
	Recommended []string
}

type Plan struct {
	RequestedCapabilities []string
	Agents                []AgentPlan
	UncoveredCapabilities []string
}

func Build(
	request Request,
	detection detector.Detection,
	agents registry.AgentRegistry,
	skills registry.SkillRegistry,
	tools registry.ToolRegistry,
) (Plan, error) {
	required := uniqueSorted(request.Capabilities)
	if len(required) == 0 {
		return Plan{}, fmt.Errorf("at least one capability is required")
	}

	for _, capability := range required {
		if capability == "" {
			return Plan{}, fmt.Errorf("capability cannot be empty")
		}
	}

	selected, uncovered := selectAgents(required, agents)
	plan := Plan{
		RequestedCapabilities: required,
		UncoveredCapabilities: uncovered,
	}

	for _, selectedAgent := range selected {
		agent := agents.Agents[selectedAgent.AgentID]
		resolution := resolver.ResolveSkills(detection, skills, resolver.SkillOptions{
			AgentID:      agent.ID,
			Capabilities: selectedAgent.Capabilities,
		})

		enabled := skillIDs(resolution.Enabled)
		candidates := skillIDs(resolution.Candidates)
		requiredTools := append([]string{}, agent.Tools.Required...)
		recommendedTools := append([]string{}, agent.Tools.Optional...)

		for _, match := range resolution.Enabled {
			skill := skills.Skills[match.SkillID]
			requiredTools = append(requiredTools, skill.Tools.Required...)
			recommendedTools = append(recommendedTools, skill.Tools.Recommended...)
		}
		for _, match := range resolution.Candidates {
			skill := skills.Skills[match.SkillID]
			requiredTools = append(requiredTools, skill.Tools.Required...)
			recommendedTools = append(recommendedTools, skill.Tools.Recommended...)
		}

		requiredTools = enabledToolIDs(uniqueSorted(requiredTools), tools)
		recommendedTools = enabledToolIDs(uniqueSorted(recommendedTools), tools)
		recommendedTools = subtract(recommendedTools, requiredTools)

		plan.Agents = append(plan.Agents, AgentPlan{
			AgentID:      agent.ID,
			Capabilities: selectedAgent.Capabilities,
			Skills: SkillPlan{
				Enabled:    enabled,
				Candidates: candidates,
			},
			Tools: ToolPlan{
				Required:    requiredTools,
				Recommended: recommendedTools,
			},
		})
	}

	sort.Slice(plan.Agents, func(i, j int) bool {
		return plan.Agents[i].AgentID < plan.Agents[j].AgentID
	})
	return plan, nil
}

type selectedAgent struct {
	AgentID      string
	Capabilities []string
}

func selectAgents(required []string, registry registry.AgentRegistry) ([]selectedAgent, []string) {
	remaining := make(map[string]bool, len(required))
	for _, capability := range required {
		remaining[capability] = true
	}

	var selected []selectedAgent
	used := map[string]bool{}

	for len(remaining) > 0 {
		bestID := ""
		var bestCoverage []string

		ids := make([]string, 0, len(registry.Agents))
		for id := range registry.Agents {
			ids = append(ids, id)
		}
		sort.Strings(ids)

		for _, id := range ids {
			agent := registry.Agents[id]
			if agent.Status != "active" || agent.Role == "orchestrator" || used[id] {
				continue
			}

			var coverage []string
			for capability := range remaining {
				if contains(agent.Capabilities, capability) {
					coverage = append(coverage, capability)
				}
			}
			sort.Strings(coverage)

			if len(coverage) > len(bestCoverage) {
				bestID = id
				bestCoverage = coverage
			}
		}

		if bestID == "" || len(bestCoverage) == 0 {
			break
		}

		selected = append(selected, selectedAgent{
			AgentID:      bestID,
			Capabilities: bestCoverage,
		})
		used[bestID] = true
		for _, capability := range bestCoverage {
			delete(remaining, capability)
		}
	}

	var uncovered []string
	for capability := range remaining {
		uncovered = append(uncovered, capability)
	}
	sort.Strings(uncovered)
	return selected, uncovered
}

func skillIDs(matches []resolver.SkillMatch) []string {
	ids := make([]string, 0, len(matches))
	for _, match := range matches {
		ids = append(ids, match.SkillID)
	}
	sort.Strings(ids)
	return ids
}

func enabledToolIDs(ids []string, registry registry.ToolRegistry) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		tool, ok := registry.Tools[id]
		if !ok || tool.Status != "enabled" {
			continue
		}
		out = append(out, id)
	}
	return out
}

func subtract(values, excluded []string) []string {
	excludedSet := map[string]bool{}
	for _, value := range excluded {
		excludedSet[value] = true
	}

	var out []string
	for _, value := range values {
		if !excludedSet[value] {
			out = append(out, value)
		}
	}
	return out
}

func uniqueSorted(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
