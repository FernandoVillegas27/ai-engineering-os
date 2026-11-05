package mutation

import (
	"strings"
	"testing"

	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/scheduler"
)

func prestageUnit(id, agent string, sourceWrite bool) execution.WorkUnit {
	return execution.WorkUnit{
		ID:      id,
		AgentID: agent,
		Mutation: execution.MutationCapability{
			SourceCode: sourceWrite,
		},
	}
}

func TestBuildPreStagePlanSelectsTransitiveReadOnlyAncestors(t *testing.T) {
	units := []execution.WorkUnit{
		prestageUnit("research", "researcher", false),
		prestageUnit("architect", "architect", false),
		prestageUnit("backend", "backend-engineer", true),
		prestageUnit("frontend", "frontend-engineer", true),
		prestageUnit("qa", "qa-engineer", false),
		prestageUnit("review", "reviewer", false),
	}
	tasks := []scheduler.Task{
		{ID: "research", AgentID: "researcher"},
		{ID: "architect", AgentID: "architect", DependsOn: []string{"research"}},
		{ID: "backend", AgentID: "backend-engineer", DependsOn: []string{"architect"}},
		{ID: "frontend", AgentID: "frontend-engineer", DependsOn: []string{"architect"}},
		{ID: "qa", AgentID: "qa-engineer", DependsOn: []string{"backend", "frontend"}},
		{ID: "review", AgentID: "reviewer", DependsOn: []string{"qa"}},
	}
	plan, err := BuildPreStagePlan(
		units,
		tasks,
		[]execution.WorkUnit{units[2], units[3]},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Units) != 2 || plan.Units[0].ID != "architect" || plan.Units[1].ID != "research" {
		t.Fatalf("units = %#v", plan.Units)
	}
	if len(plan.Tasks) != 2 {
		t.Fatalf("tasks = %#v", plan.Tasks)
	}
	var architect scheduler.Task
	for _, task := range plan.Tasks {
		if task.ID == "architect" {
			architect = task
		}
	}
	if len(architect.DependsOn) != 1 || architect.DependsOn[0] != "research" {
		t.Fatalf("architect = %#v", architect)
	}
	if len(plan.Dependents["architect"]) != 2 {
		t.Fatalf("full dependents lost mutating targets: %#v", plan.Dependents["architect"])
	}
	if _, ok := plan.Dependents["backend"]; !ok {
		t.Fatal("full graph dependents should retain downstream QA")
	}
}

func TestBuildPreStagePlanAllowsNoReadOnlyAncestors(t *testing.T) {
	unit := prestageUnit("backend", "backend-engineer", true)
	plan, err := BuildPreStagePlan(
		[]execution.WorkUnit{unit},
		[]scheduler.Task{{ID: unit.ID, AgentID: unit.AgentID}},
		[]execution.WorkUnit{unit},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Units) != 0 || len(plan.Tasks) != 0 {
		t.Fatalf("plan = %#v", plan)
	}
}

func TestBuildPreStagePlanRejectsReadOnlyTaskDependingOnMutation(t *testing.T) {
	units := []execution.WorkUnit{
		prestageUnit("backend-a", "backend-engineer", true),
		prestageUnit("architect", "architect", false),
		prestageUnit("backend-b", "frontend-engineer", true),
	}
	tasks := []scheduler.Task{
		{ID: "backend-a", AgentID: "backend-engineer"},
		{ID: "architect", AgentID: "architect", DependsOn: []string{"backend-a"}},
		{ID: "backend-b", AgentID: "frontend-engineer", DependsOn: []string{"architect"}},
	}
	_, err := BuildPreStagePlan(
		units,
		tasks,
		[]execution.WorkUnit{units[0], units[2]},
	)
	if err == nil || !strings.Contains(err.Error(), "read-only task architect depends on mutating task backend-a") {
		t.Fatalf("err = %#v", err)
	}
}

func TestBuildPreStagePlanRejectsMutatingUnitOutsideFullPlan(t *testing.T) {
	all := []execution.WorkUnit{prestageUnit("architect", "architect", false)}
	_, err := BuildPreStagePlan(
		all,
		[]scheduler.Task{{ID: "architect", AgentID: "architect"}},
		[]execution.WorkUnit{prestageUnit("backend", "backend-engineer", true)},
	)
	if err == nil {
		t.Fatal("expected unknown mutating unit rejection")
	}
}
