package execution

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/FernandoVillegas27/ai-engineering-os/core/orchestration"
	"github.com/FernandoVillegas27/ai-engineering-os/core/registry"
)

type Mode string

const (
	ModeReadOnly Mode = "read-only"
)

type PermissionEnvelope struct {
	SourceCode string
	Tests      string
	GitCommit  string
	GitPush    string
	Secrets    string
	Production string
}

type MutationCapability struct {
	SourceCode bool
	Tests      bool
}

type WorkUnit struct {
	ID           string
	Task         string
	ProjectRoot  string
	Intent       orchestration.Intent
	AgentID      string
	Capabilities []string
	Skills       []string
	Tools                []string
	ToolOperations       map[string][]string
	CollaborationContext string
	Permissions          PermissionEnvelope
	Mutation             MutationCapability
	Mode                 Mode
}

func BuildWorkUnits(
	task string,
	projectRoot string,
	plan orchestration.Plan,
	agents registry.AgentRegistry,
) ([]WorkUnit, error) {
	if len(plan.UncoveredCapabilities) > 0 {
		return nil, fmt.Errorf("cannot create work units with uncovered capabilities: %v", plan.UncoveredCapabilities)
	}
	if len(plan.Agents) == 0 {
		return nil, fmt.Errorf("execution plan has no agents")
	}

	root, err := filepath.Abs(projectRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve project root: %w", err)
	}

	units := make([]WorkUnit, 0, len(plan.Agents))
	for index, agentPlan := range plan.Agents {
		agent, ok := agents.Agents[agentPlan.AgentID]
		if !ok {
			return nil, fmt.Errorf("agent %q not found in registry", agentPlan.AgentID)
		}
		if agent.Status != "active" {
			return nil, fmt.Errorf("agent %q is not active", agentPlan.AgentID)
		}

		tools := append([]string{}, agentPlan.Tools.Required...)
		tools = append(tools, agentPlan.Tools.Recommended...)

		units = append(units, WorkUnit{
			ID:           fmt.Sprintf("work-%02d-%s", index+1, agentPlan.AgentID),
			Task:         task,
			ProjectRoot:  root,
			Intent:       plan.Intent,
			AgentID:      agentPlan.AgentID,
			Capabilities: uniqueSorted(agentPlan.Capabilities),
			Skills:       uniqueSorted(agentPlan.Skills.Enabled),
			Tools:          uniqueSorted(tools),
			ToolOperations: cloneToolOperations(agentPlan.Tools.Operations),
			Permissions:    readOnlyEnvelope(agent.Permissions),
			Mutation:     mutationCapability(agent.Permissions),
			Mode:         ModeReadOnly,
		})
	}

	return units, nil
}

func readOnlyEnvelope(source registry.AgentPermissions) PermissionEnvelope {
	sourceCode := "deny"
	if source.SourceCode == "read" || source.SourceCode == "write" {
		sourceCode = "read"
	}
	tests := "deny"
	if source.Tests == "read" || source.Tests == "write" {
		tests = "read"
	}

	return PermissionEnvelope{
		SourceCode: sourceCode,
		Tests:      tests,
		GitCommit:  "deny",
		GitPush:    "deny",
		Secrets:    "deny",
		Production: "deny",
	}
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


func mutationCapability(source registry.AgentPermissions) MutationCapability {
	return MutationCapability{
		SourceCode: source.SourceCode == "write",
		Tests:      source.Tests == "write",
	}
}

func (u WorkUnit) CanMutate() bool {
	return u.Mutation.SourceCode || u.Mutation.Tests
}


func cloneToolOperations(values map[string][]string) map[string][]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string][]string, len(values))
	for toolID, operations := range values {
		out[toolID] = uniqueSorted(operations)
	}
	return out
}


func FormatToolBindings(unit WorkUnit) string {
	if len(unit.Tools) == 0 {
		return ""
	}
	parts := make([]string, 0, len(unit.Tools))
	for _, toolID := range uniqueSorted(unit.Tools) {
		operations := uniqueSorted(unit.ToolOperations[toolID])
		if len(operations) == 0 {
			parts = append(parts, toolID+"[no-bound-operations]")
			continue
		}
		parts = append(parts, toolID+"["+strings.Join(operations, ",")+"]")
	}
	return strings.Join(parts, "; ")
}
