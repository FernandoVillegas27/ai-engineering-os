package taskgraph

import (
	"reflect"
	"testing"

	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/scheduler"
)

func testUnit(id, agent string) execution.WorkUnit {
	return execution.WorkUnit{ID: id, AgentID: agent, Mode: execution.ModeReadOnly}
}

func depsByAgent(tasks []scheduler.Task) map[string][]string {
	out := map[string][]string{}
	for _, task := range tasks {
		out[task.AgentID] = append([]string(nil), task.DependsOn...)
	}
	return out
}

func TestBuildCreatesSpecialistCollaborationOrder(t *testing.T) {
	tasks, err := Build([]execution.WorkUnit{
		testUnit("work-01-researcher", "researcher"),
		testUnit("work-02-architect", "architect"),
		testUnit("work-03-backend", "backend-engineer"),
		testUnit("work-04-frontend", "frontend-engineer"),
		testUnit("work-05-database", "database-engineer"),
		testUnit("work-06-qa", "qa-engineer"),
		testUnit("work-07-reviewer", "reviewer"),
	})
	if err != nil {
		t.Fatal(err)
	}

	deps := depsByAgent(tasks)
	if len(deps["researcher"]) != 0 {
		t.Fatalf("researcher deps = %#v", deps["researcher"])
	}
	if !reflect.DeepEqual(deps["architect"], []string{"work-01-researcher"}) {
		t.Fatalf("architect deps = %#v", deps["architect"])
	}
	if !reflect.DeepEqual(deps["backend-engineer"], []string{"work-02-architect"}) {
		t.Fatalf("backend deps = %#v", deps["backend-engineer"])
	}
	if !reflect.DeepEqual(deps["frontend-engineer"], []string{"work-02-architect"}) {
		t.Fatalf("frontend deps = %#v", deps["frontend-engineer"])
	}
	if !reflect.DeepEqual(deps["database-engineer"], []string{"work-02-architect"}) {
		t.Fatalf("database deps = %#v", deps["database-engineer"])
	}
	if !reflect.DeepEqual(deps["qa-engineer"], []string{
		"work-03-backend",
		"work-04-frontend",
		"work-05-database",
	}) {
		t.Fatalf("qa deps = %#v", deps["qa-engineer"])
	}
	if !reflect.DeepEqual(deps["reviewer"], []string{
		"work-01-researcher",
		"work-02-architect",
		"work-03-backend",
		"work-04-frontend",
		"work-05-database",
		"work-06-qa",
	}) {
		t.Fatalf("reviewer deps = %#v", deps["reviewer"])
	}
}

func TestBuildAdaptsWhenRolesAreMissing(t *testing.T) {
	tasks, err := Build([]execution.WorkUnit{
		testUnit("work-backend", "backend-engineer"),
		testUnit("work-qa", "qa-engineer"),
		testUnit("work-review", "reviewer"),
	})
	if err != nil {
		t.Fatal(err)
	}
	deps := depsByAgent(tasks)
	if len(deps["backend-engineer"]) != 0 {
		t.Fatalf("backend deps = %#v", deps["backend-engineer"])
	}
	if !reflect.DeepEqual(deps["qa-engineer"], []string{"work-backend"}) {
		t.Fatalf("qa deps = %#v", deps["qa-engineer"])
	}
	if !reflect.DeepEqual(deps["reviewer"], []string{"work-backend", "work-qa"}) {
		t.Fatalf("reviewer deps = %#v", deps["reviewer"])
	}
}

func TestBuildUsesResearchAsFallbackInputWithoutArchitect(t *testing.T) {
	tasks, err := Build([]execution.WorkUnit{
		testUnit("work-research", "researcher"),
		testUnit("work-backend", "backend-engineer"),
	})
	if err != nil {
		t.Fatal(err)
	}
	deps := depsByAgent(tasks)
	if !reflect.DeepEqual(deps["backend-engineer"], []string{"work-research"}) {
		t.Fatalf("backend deps = %#v", deps["backend-engineer"])
	}
}

func TestBuildKeepsUnknownFutureRolesIndependent(t *testing.T) {
	tasks, err := Build([]execution.WorkUnit{
		testUnit("work-security", "security-engineer"),
		testUnit("work-backend", "backend-engineer"),
	})
	if err != nil {
		t.Fatal(err)
	}
	deps := depsByAgent(tasks)
	if len(deps["security-engineer"]) != 0 {
		t.Fatalf("security deps = %#v", deps["security-engineer"])
	}
}

func TestBuildRejectsDuplicateAgentWorkUnits(t *testing.T) {
	_, err := Build([]execution.WorkUnit{
		testUnit("work-a", "backend-engineer"),
		testUnit("work-b", "backend-engineer"),
	})
	if err == nil {
		t.Fatal("expected duplicate agent rejection")
	}
}

func TestBuildRejectsEmptyInput(t *testing.T) {
	if _, err := Build(nil); err == nil {
		t.Fatal("expected empty graph rejection")
	}
}
