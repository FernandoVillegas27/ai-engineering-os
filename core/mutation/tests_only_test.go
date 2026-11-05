package mutation

import (
	"testing"

	"github.com/FernandoVillegas27/ai-engineering-os/core/staging"
)

func TestValidateFrozenTestsOnlyAcceptsTestPaths(t *testing.T) {
	values := []FrozenProposal{
		{
			TaskID:  "qa",
			AgentID: "qa-engineer",
			Declared: []staging.WorkspaceChange{
				{Path: "pkg/service_test.go", Action: "modify"},
				{Path: "frontend/app.spec.ts", Action: "create"},
			},
		},
	}
	if err := ValidateFrozenTestsOnly(values); err != nil {
		t.Fatal(err)
	}
}

func TestValidateFrozenTestsOnlyRejectsSourceOrMixedScope(t *testing.T) {
	cases := [][]FrozenProposal{
		{{
			TaskID: "qa", AgentID: "qa-engineer",
			Declared: []staging.WorkspaceChange{{Path: "pkg/service.go", Action: "modify"}},
		}},
		{{
			TaskID: "qa", AgentID: "qa-engineer",
			Declared: []staging.WorkspaceChange{
				{Path: "pkg/service_test.go", Action: "modify"},
				{Path: "pkg/service.go", Action: "modify"},
			},
		}},
	}
	for _, values := range cases {
		if err := ValidateFrozenTestsOnly(values); err == nil {
			t.Fatalf("expected tests-only rejection for %#v", values)
		}
	}
}
