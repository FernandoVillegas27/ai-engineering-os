package orchestration

import (
	"fmt"
	"sort"

	"github.com/FernandoVillegas27/ai-engineering-os/core/detector"
	"github.com/FernandoVillegas27/ai-engineering-os/core/registry"
	"github.com/FernandoVillegas27/ai-engineering-os/core/resolver"
)

type Intent string

const (
	IntentImplement Intent = "implement"
	IntentDesign    Intent = "design"
	IntentResearch  Intent = "research"
	IntentReview    Intent = "review"
	IntentVerify    Intent = "verify"
)

type Request struct {
	Capabilities []string
	Intent       Intent
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
	Intent                Intent
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

	intent := request.Intent
	if intent == "" {
		intent = IntentImplement
	}
	if !validIntent(intent) {
		return Plan{}, fmt.Errorf("unsupported execution intent %q", intent)
	}

	selected, uncovered := selectAgents(required, intent, agents)
	plan := Plan{
		Intent:                intent,
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

		requiredTools, err = resolveRequiredTools(
			agent.ID,
			uniqueSorted(requiredTools),
			tools,
		)
		if err != nil {
			return Plan{}, err
		}
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

type agentCandidate struct {
	ID             string
	Coverage       []string
	IntentScore    int
	Specialization int
}

func selectAgents(required []string, intent Intent, registry registry.AgentRegistry) ([]selectedAgent, []string) {
	remaining := make(map[string]bool, len(required))
	for _, capability := range required {
		remaining[capability] = true
	}

	var selected []selectedAgent
	used := map[string]bool{}

	for len(remaining) > 0 {
		var best *agentCandidate

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
			if !eligibleForIntent(agent, intent) {
				continue
			}

			var coverage []string
			for capability := range remaining {
				if contains(agent.Capabilities, capability) {
					coverage = append(coverage, capability)
				}
			}
			sort.Strings(coverage)
			if len(coverage) == 0 {
				continue
			}

			candidate := agentCandidate{
				ID:             id,
				Coverage:       coverage,
				IntentScore:    intentScore(agent, intent),
				Specialization: specializationScore(agent, coverage),
			}
			if best == nil || betterCandidate(candidate, *best) {
				copy := candidate
				best = &copy
			}
		}

		if best == nil {
			break
		}

		selected = append(selected, selectedAgent{
			AgentID:      best.ID,
			Capabilities: best.Coverage,
		})
		used[best.ID] = true
		for _, capability := range best.Coverage {
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

func betterCandidate(left, right agentCandidate) bool {
	if len(left.Coverage) != len(right.Coverage) {
		return len(left.Coverage) > len(right.Coverage)
	}
	if left.IntentScore != right.IntentScore {
		return left.IntentScore > right.IntentScore
	}
	if left.Specialization != right.Specialization {
		return left.Specialization > right.Specialization
	}
	return left.ID < right.ID
}

func eligibleForIntent(agent registry.Agent, intent Intent) bool {
	switch intent {
	case IntentImplement:
		return agent.Permissions.SourceCode == "write" || agent.Permissions.Tests == "write"
	case IntentDesign, IntentResearch, IntentReview:
		return agent.Permissions.SourceCode == "read" || agent.Permissions.SourceCode == "write"
	case IntentVerify:
		return agent.Permissions.Tests == "read" || agent.Permissions.Tests == "write"
	default:
		return false
	}
}

func intentScore(agent registry.Agent, intent Intent) int {
	score := 0

	switch intent {
	case IntentImplement:
		if agent.Permissions.SourceCode == "write" {
			score += 40
		}
		if agent.Permissions.Tests == "write" {
			score += 20
		}
		if agent.Role == "frontend-engineer" || agent.Role == "backend-engineer" || agent.Role == "database-engineer" {
			score += 20
		}
	case IntentDesign:
		if agent.Role == "architect" {
			score += 60
		}
		if agent.Role == "database-engineer" {
			score += 25
		}
		if agent.Permissions.SourceCode == "read" {
			score += 10
		}
	case IntentResearch:
		if agent.Role == "researcher" {
			score += 60
		}
		if contains(agent.SkillDomains, "research") {
			score += 20
		}
	case IntentReview:
		if agent.Role == "reviewer" {
			score += 60
		}
		if agent.Permissions.GitCommit == "deny" && agent.Permissions.GitPush == "deny" {
			score += 15
		}
		if contains(agent.Capabilities, "code-review") {
			score += 10
		}
	case IntentVerify:
		if agent.Role == "qa-engineer" {
			score += 60
		}
		if agent.Permissions.Tests == "write" {
			score += 25
		}
		if contains(agent.Capabilities, "automated-testing") {
			score += 10
		}
	}

	return score
}

func specializationScore(agent registry.Agent, coverage []string) int {
	if len(agent.Capabilities) == 0 {
		return 0
	}
	return len(coverage)*100 - len(agent.Capabilities)
}

func validIntent(intent Intent) bool {
	switch intent {
	case IntentImplement, IntentDesign, IntentResearch, IntentReview, IntentVerify:
		return true
	default:
		return false
	}
}

func skillIDs(matches []resolver.SkillMatch) []string {
	ids := make([]string, 0, len(matches))
	for _, match := range matches {
		ids = append(ids, match.SkillID)
	}
	sort.Strings(ids)
	return ids
}

func resolveRequiredTools(
	agentID string,
	ids []string,
	registry registry.ToolRegistry,
) ([]string, error) {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		tool, ok := registry.Tools[id]
		if !ok {
			return nil, fmt.Errorf(
				"agent %q requires tool %q but it is not present in the tool registry",
				agentID,
				id,
			)
		}
		if tool.Status != "enabled" {
			return nil, fmt.Errorf(
				"agent %q requires tool %q but its registry status is %q",
				agentID,
				id,
				tool.Status,
			)
		}
		out = append(out, id)
	}
	return out, nil
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
