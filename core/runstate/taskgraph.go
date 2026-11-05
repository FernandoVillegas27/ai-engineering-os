package runstate

import (
	"errors"
	"fmt"
	"sort"

	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/scheduler"
)

// BindTaskGraph persists the deterministic scheduler graph into durable run
// state without changing task identity, permissions or provider configuration.
func BindTaskGraph(run *Run, tasks []scheduler.Task) error {
	if run == nil {
		return errors.New("run is required")
	}
	if len(tasks) == 0 {
		return errors.New("task graph is required")
	}
	byID := make(map[string]scheduler.Task, len(tasks))
	for _, task := range tasks {
		if _, exists := byID[task.ID]; exists {
			return fmt.Errorf("duplicate task graph id: %s", task.ID)
		}
		byID[task.ID] = task
	}
	if len(byID) != len(run.WorkUnits) {
		return fmt.Errorf(
			"task graph/work-unit count mismatch: graph=%d run=%d",
			len(byID),
			len(run.WorkUnits),
		)
	}
	for index := range run.WorkUnits {
		unit := &run.WorkUnits[index]
		task, ok := byID[unit.ID]
		if !ok {
			return fmt.Errorf("task graph is missing run work unit %s", unit.ID)
		}
		if task.AgentID != unit.AgentID {
			return fmt.Errorf(
				"task graph agent mismatch for %s: graph=%s run=%s",
				unit.ID,
				task.AgentID,
				unit.AgentID,
			)
		}
		unit.DependsOn = append([]string(nil), task.DependsOn...)
		sort.Strings(unit.DependsOn)
		unit.Mutating = task.Mutating
		unit.Scopes = append([]string(nil), task.Scopes...)
		sort.Strings(unit.Scopes)
		unit.Status = schedulerToWorkUnitStatus(task.Status)
	}
	return Validate(*run)
}

func schedulerToWorkUnitStatus(status scheduler.Status) WorkUnitStatus {
	switch status {
	case scheduler.StatusQueued:
		return WorkUnitQueued
	case scheduler.StatusReady:
		return WorkUnitReady
	case scheduler.StatusRunning:
		return WorkUnitRunning
	case scheduler.StatusWaiting:
		return WorkUnitWaiting
	case scheduler.StatusBlocked:
		return WorkUnitBlocked
	case scheduler.StatusCompleted:
		return WorkUnitCompleted
	case scheduler.StatusFailed:
		return WorkUnitFailed
	case scheduler.StatusCancelled:
		return WorkUnitCancelled
	default:
		return WorkUnitQueued
	}
}


func BindMutationKinds(run *Run, units []execution.WorkUnit) error {
	if run == nil {
		return errors.New("run is required")
	}
	byID := map[string]execution.WorkUnit{}
	for _, unit := range units {
		if unit.ID == "" {
			return errors.New("mutation-kind work unit id is required")
		}
		if _, exists := byID[unit.ID]; exists {
			return fmt.Errorf("duplicate mutation-kind work unit: %s", unit.ID)
		}
		byID[unit.ID] = unit
	}
	for index := range run.WorkUnits {
		durable := &run.WorkUnits[index]
		if !durable.Mutating {
			durable.MutationKind = ""
			continue
		}
		unit, ok := byID[durable.ID]
		if !ok {
			return fmt.Errorf("missing mutation capability metadata for %s", durable.ID)
		}
		switch {
		case unit.Mutation.SourceCode:
			durable.MutationKind = "source"
		case unit.Mutation.Tests:
			durable.MutationKind = "tests-only"
		default:
			return fmt.Errorf("mutating work unit %s has no registry-derived write capability", durable.ID)
		}
	}
	return Validate(*run)
}
