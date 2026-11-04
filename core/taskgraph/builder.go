package taskgraph

import (
	"fmt"
	"sort"

	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/scheduler"
)

var implementationRoles = map[string]bool{
	"backend-engineer":  true,
	"frontend-engineer": true,
	"database-engineer": true,
}

func Build(units []execution.WorkUnit) ([]scheduler.Task, error) {
	if len(units) == 0 {
		return nil, fmt.Errorf("task graph requires at least one work unit")
	}

	byAgent := map[string]execution.WorkUnit{}
	for _, unit := range units {
		if unit.ID == "" || unit.AgentID == "" {
			return nil, fmt.Errorf("work units require id and agent id")
		}
		if _, exists := byAgent[unit.AgentID]; exists {
			return nil, fmt.Errorf("duplicate agent work unit: %s", unit.AgentID)
		}
		byAgent[unit.AgentID] = unit
	}

	researcherID := idFor(byAgent, "researcher")
	architectID := idFor(byAgent, "architect")

	var implementationIDs []string
	for role := range implementationRoles {
		if id := idFor(byAgent, role); id != "" {
			implementationIDs = append(implementationIDs, id)
		}
	}
	sort.Strings(implementationIDs)

	tasks := make([]scheduler.Task, 0, len(units))
	for _, unit := range units {
		var dependencies []string
		switch unit.AgentID {
		case "researcher":
			// Research is an input into later architecture/implementation work.
		case "architect":
			dependencies = appendIf(dependencies, researcherID)
		case "backend-engineer", "frontend-engineer", "database-engineer":
			dependencies = appendIf(dependencies, architectID)
			if architectID == "" {
				dependencies = appendIf(dependencies, researcherID)
			}
		case "qa-engineer":
			dependencies = append(dependencies, implementationIDs...)
			if len(implementationIDs) == 0 {
				dependencies = appendIf(dependencies, architectID)
				if architectID == "" {
					dependencies = appendIf(dependencies, researcherID)
				}
			}
		case "reviewer":
			for _, other := range units {
				if other.ID != unit.ID {
					dependencies = append(dependencies, other.ID)
				}
			}
		default:
			// Unknown future roles remain independent rather than inheriting
			// guessed dependencies. Role-specific ordering should be explicit.
		}

		dependencies = uniqueSorted(dependencies)
		tasks = append(tasks, scheduler.Task{
			ID:        unit.ID,
			AgentID:   unit.AgentID,
			DependsOn: dependencies,
			Mutating:  false,
		})
	}

	sort.Slice(tasks, func(i, j int) bool { return tasks[i].ID < tasks[j].ID })

	// Let the scheduler validate cycles/unknown dependencies now so callers
	// never receive a structurally invalid graph.
	if _, err := scheduler.New(tasks); err != nil {
		return nil, fmt.Errorf("validate specialist collaboration graph: %w", err)
	}
	return tasks, nil
}

func idFor(units map[string]execution.WorkUnit, agentID string) string {
	if unit, ok := units[agentID]; ok {
		return unit.ID
	}
	return ""
}

func appendIf(values []string, value string) []string {
	if value == "" {
		return values
	}
	return append(values, value)
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
