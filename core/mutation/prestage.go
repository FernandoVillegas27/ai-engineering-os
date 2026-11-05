package mutation

import (
	"errors"
	"fmt"
	"sort"

	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/scheduler"
)

type PreStagePlan struct {
	Units      []execution.WorkUnit
	Tasks      []scheduler.Task
	Dependents map[string][]scheduler.Task
}

func BuildPreStagePlan(
	allUnits []execution.WorkUnit,
	fullTasks []scheduler.Task,
	mutatingUnits []execution.WorkUnit,
) (PreStagePlan, error) {
	if len(allUnits) == 0 || len(fullTasks) == 0 || len(mutatingUnits) == 0 {
		return PreStagePlan{}, errors.New("pre-stage collaboration requires full units, full task graph, and mutating units")
	}

	unitByID := map[string]execution.WorkUnit{}
	for _, unit := range allUnits {
		if unit.ID == "" {
			return PreStagePlan{}, errors.New("pre-stage unit id is required")
		}
		if _, exists := unitByID[unit.ID]; exists {
			return PreStagePlan{}, fmt.Errorf("duplicate pre-stage unit id: %s", unit.ID)
		}
		unitByID[unit.ID] = unit
	}

	taskByID := map[string]scheduler.Task{}
	for _, task := range fullTasks {
		if _, ok := unitByID[task.ID]; !ok {
			return PreStagePlan{}, fmt.Errorf("task graph references unknown unit %s", task.ID)
		}
		taskByID[task.ID] = task
	}
	if len(taskByID) != len(unitByID) {
		return PreStagePlan{}, errors.New("task graph does not cover every work unit")
	}

	mutating := map[string]bool{}
	for _, unit := range mutatingUnits {
		full, ok := unitByID[unit.ID]
		if !ok {
			return PreStagePlan{}, fmt.Errorf("mutating unit %s is not in the full plan", unit.ID)
		}
		if full.AgentID != unit.AgentID {
			return PreStagePlan{}, fmt.Errorf("mutating unit %s agent mismatch", unit.ID)
		}
		if !unit.Mutation.SourceCode {
			return PreStagePlan{}, fmt.Errorf("mutating unit %s is not source-write capable", unit.ID)
		}
		mutating[unit.ID] = true
	}

	selected := map[string]bool{}
	visiting := map[string]bool{}
	var visit func(string) error
	visit = func(id string) error {
		if visiting[id] {
			return fmt.Errorf("pre-stage dependency cycle at %s", id)
		}
		task, ok := taskByID[id]
		if !ok {
			return fmt.Errorf("unknown dependency task %s", id)
		}
		visiting[id] = true
		defer delete(visiting, id)

		for _, dep := range task.DependsOn {
			depUnit := unitByID[dep]
			if mutating[dep] || depUnit.Mutation.SourceCode {
				// A read-only pre-stage task cannot wait on source mutation. The
				// current phase model requires all advisory work to precede mutation.
				if !mutating[id] {
					return fmt.Errorf(
						"unsupported pre-stage order: read-only task %s depends on mutating task %s",
						id,
						dep,
					)
				}
				if err := visit(dep); err != nil {
					return err
				}
				continue
			}
			selected[dep] = true
			if err := visit(dep); err != nil {
				return err
			}
		}
		return nil
	}
	for id := range mutating {
		if err := visit(id); err != nil {
			return PreStagePlan{}, err
		}
	}

	if len(selected) == 0 {
		return PreStagePlan{
			Units:      []execution.WorkUnit{},
			Tasks:      []scheduler.Task{},
			Dependents: buildDependents(fullTasks),
		}, nil
	}

	var units []execution.WorkUnit
	for id := range selected {
		unit := unitByID[id]
		if unit.Mutation.SourceCode {
			return PreStagePlan{}, fmt.Errorf("pre-stage selected mutating unit %s", id)
		}
		units = append(units, unit)
	}
	sort.Slice(units, func(i, j int) bool { return units[i].ID < units[j].ID })

	var tasks []scheduler.Task
	for _, unit := range units {
		full := taskByID[unit.ID]
		task := scheduler.Task{
			ID:       full.ID,
			AgentID:  full.AgentID,
			Mutating: false,
			Status:   scheduler.StatusQueued,
		}
		for _, dep := range full.DependsOn {
			if selected[dep] {
				task.DependsOn = append(task.DependsOn, dep)
				continue
			}
			if mutating[dep] || unitByID[dep].Mutation.SourceCode {
				return PreStagePlan{}, fmt.Errorf(
					"unsupported pre-stage order: selected read-only task %s depends on mutating task %s",
					unit.ID,
					dep,
				)
			}
		}
		sort.Strings(task.DependsOn)
		tasks = append(tasks, task)
	}
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].ID < tasks[j].ID })
	if _, err := scheduler.New(tasks); err != nil {
		return PreStagePlan{}, fmt.Errorf("validate pre-stage graph: %w", err)
	}

	return PreStagePlan{
		Units:      units,
		Tasks:      tasks,
		Dependents: buildDependents(fullTasks),
	}, nil
}

func buildDependents(tasks []scheduler.Task) map[string][]scheduler.Task {
	out := map[string][]scheduler.Task{}
	for _, task := range tasks {
		for _, dep := range task.DependsOn {
			out[dep] = append(out[dep], task)
		}
	}
	for id := range out {
		sort.Slice(out[id], func(i, j int) bool {
			return out[id][i].ID < out[id][j].ID
		})
	}
	return out
}
