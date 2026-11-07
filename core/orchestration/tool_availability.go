package orchestration

import (
	"fmt"

	"github.com/FernandoVillegas27/ai-engineering-os/core/registry"
	"github.com/FernandoVillegas27/ai-engineering-os/core/toolavailability"
)

// ApplyToolAvailability narrows a declarative plan using runtime evidence.
// It can only remove optional capabilities or block unavailable required ones;
// it never adds a tool that was absent from the original plan.
func ApplyToolAvailability(
	plan Plan,
	tools registry.ToolRegistry,
	snapshot toolavailability.Snapshot,
) (Plan, error) {
	if snapshot == nil {
		return plan, nil
	}
	if err := toolavailability.ValidateSnapshot(tools, snapshot); err != nil {
		return Plan{}, fmt.Errorf("validate runtime tool availability: %w", err)
	}

	out := plan
	out.Agents = append([]AgentPlan(nil), plan.Agents...)
	for index, agent := range out.Agents {
		required, recommended, err := toolavailability.ResolvePlan(
			agent.Tools.Required,
			agent.Tools.Recommended,
			tools,
			snapshot,
		)
		if err != nil {
			return Plan{}, fmt.Errorf("agent %q runtime tools: %w", agent.AgentID, err)
		}
		agent.Tools.Required = required
		agent.Tools.Recommended = recommended
		out.Agents[index] = agent
	}
	return out, nil
}
