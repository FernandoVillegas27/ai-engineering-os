package mutation

import (
	"testing"

	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
)

func TestSelectSourceMutatingUnitsFiltersByRegistryDerivedMetadata(t *testing.T) {
	units := []execution.WorkUnit{
		{ID: "architect", AgentID: "architect"},
		{ID: "backend", AgentID: "backend-engineer", Mutation: execution.MutationCapability{SourceCode: true, Tests: true}},
		{ID: "frontend", AgentID: "frontend-engineer", Mutation: execution.MutationCapability{SourceCode: true, Tests: true}},
		{ID: "database", AgentID: "database-engineer", Mutation: execution.MutationCapability{SourceCode: true, Tests: true}},
		{ID: "qa", AgentID: "qa-engineer", Mutation: execution.MutationCapability{Tests: true}},
		{ID: "reviewer", AgentID: "reviewer"},
	}
	selected, err := SelectSourceMutatingUnits(units)
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) != 3 {
		t.Fatalf("selected = %#v", selected)
	}
	if selected[0].ID != "backend" || selected[1].ID != "database" || selected[2].ID != "frontend" {
		t.Fatalf("selected = %#v", selected)
	}
	for _, unit := range selected {
		if !unit.Mutation.SourceCode {
			t.Fatalf("non-source writer selected: %#v", unit)
		}
	}
}

func TestSelectSourceMutatingUnitsRejectsReadOnlyOrTestsOnlySet(t *testing.T) {
	_, err := SelectSourceMutatingUnits([]execution.WorkUnit{
		{ID: "architect", AgentID: "architect"},
		{ID: "qa", AgentID: "qa-engineer", Mutation: execution.MutationCapability{Tests: true}},
	})
	if err == nil {
		t.Fatal("expected no source-code-write units rejection")
	}
}


func TestSelectTestsOnlyUnitsKeepsTestsWriteWithoutSourceWrite(t *testing.T) {
	units := []execution.WorkUnit{
		{ID: "backend", AgentID: "backend-engineer", Mutation: execution.MutationCapability{SourceCode: true, Tests: true}},
		{ID: "qa", AgentID: "qa-engineer", Mutation: execution.MutationCapability{Tests: true}},
		{ID: "reviewer", AgentID: "reviewer"},
		{ID: "test-auditor", AgentID: "test-auditor", Mutation: execution.MutationCapability{Tests: true}},
	}
	selected := SelectTestsOnlyUnits(units)
	if len(selected) != 2 || selected[0].ID != "qa" || selected[1].ID != "test-auditor" {
		t.Fatalf("selected = %#v", selected)
	}
	for _, unit := range selected {
		if !unit.Mutation.Tests || unit.Mutation.SourceCode {
			t.Fatalf("invalid tests-only selection: %#v", unit)
		}
	}
}
