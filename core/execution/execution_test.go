package execution

import (
	"context"
	"reflect"
	"testing"

	"github.com/FernandoVillegas27/ai-engineering-os/core/orchestration"
	"github.com/FernandoVillegas27/ai-engineering-os/core/registry"
)

func fixtureAgents() registry.AgentRegistry {
	return registry.AgentRegistry{Agents: map[string]registry.Agent{
		"reviewer": {
			ID:     "reviewer",
			Status: "active",
			Permissions: registry.AgentPermissions{
				SourceCode: "read",
				Tests:      "read",
				GitCommit:  "deny",
				GitPush:    "deny",
				Secrets:    "deny",
				Production: "deny",
			},
		},
		"backend-engineer": {
			ID:     "backend-engineer",
			Status: "active",
			Permissions: registry.AgentPermissions{
				SourceCode: "write",
				Tests:      "write",
				GitCommit:  "ask",
				GitPush:    "ask",
				Secrets:    "deny",
				Production: "deny",
			},
		},
	}}
}

func TestBuildWorkUnitsUsesOnlyEnabledSkills(t *testing.T) {
	plan := orchestration.Plan{
		Intent: orchestration.IntentReview,
		Agents: []orchestration.AgentPlan{
			{
				AgentID:      "reviewer",
				Capabilities: []string{"security-review", "code-review"},
				Skills: orchestration.SkillPlan{
					Enabled:    []string{"secure-review"},
					Candidates: []string{"untrusted-community-skill"},
				},
				Tools: orchestration.ToolPlan{
					Required:    []string{"filesystem"},
					Recommended: []string{"github"},
					Operations: map[string][]string{
						"filesystem": {"read-files"},
						"github": {"read-repository"},
					},
				},
			},
		},
	}

	units, err := BuildWorkUnits("Review permissions", ".", plan, fixtureAgents())
	if err != nil {
		t.Fatalf("BuildWorkUnits() error = %v", err)
	}
	if len(units) != 1 {
		t.Fatalf("units = %d", len(units))
	}
	if !reflect.DeepEqual(units[0].Skills, []string{"secure-review"}) {
		t.Fatalf("skills = %#v", units[0].Skills)
	}
	if got := FormatToolBindings(units[0]); got != "filesystem[read-files]; github[read-repository]" {
		t.Fatalf("tool bindings = %q", got)
	}
}

func TestBuildWorkUnitsNarrowsWriteAgentToReadOnly(t *testing.T) {
	plan := orchestration.Plan{
		Intent: orchestration.IntentImplement,
		Agents: []orchestration.AgentPlan{{
			AgentID:      "backend-engineer",
			Capabilities: []string{"backend-development"},
		}},
	}

	units, err := BuildWorkUnits("Implement service", ".", plan, fixtureAgents())
	if err != nil {
		t.Fatalf("BuildWorkUnits() error = %v", err)
	}
	got := units[0].Permissions
	if got.SourceCode != "read" || got.Tests != "read" {
		t.Fatalf("permissions = %#v", got)
	}
	if got.GitCommit != "deny" || got.GitPush != "deny" {
		t.Fatalf("git permissions = %#v", got)
	}
	if !units[0].CanMutate() || !units[0].Mutation.SourceCode || !units[0].Mutation.Tests {
		t.Fatalf("mutation capability = %#v", units[0].Mutation)
	}
}

func TestBuildWorkUnitsRejectsUncoveredCapabilities(t *testing.T) {
	_, err := BuildWorkUnits("x", ".", orchestration.Plan{
		Intent:                orchestration.IntentReview,
		UncoveredCapabilities: []string{"unknown"},
	}, fixtureAgents())
	if err == nil {
		t.Fatal("expected uncovered capability error")
	}
}

func TestDryRunExecutorDoesNotExecuteOrChangeProject(t *testing.T) {
	result, err := (DryRunExecutor{}).Execute(context.Background(), WorkUnit{
		ID:      "work-01-reviewer",
		AgentID: "reviewer",
		Mode:    ModeReadOnly,
		Permissions: PermissionEnvelope{
			SourceCode: "read",
			Tests:      "read",
			GitCommit:  "deny",
			GitPush:    "deny",
			Secrets:    "deny",
			Production: "deny",
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Executed {
		t.Fatal("dry-run must not invoke runtime execution")
	}
	if len(result.Changes) != 0 {
		t.Fatalf("changes = %#v", result.Changes)
	}
	if result.Status != StatusDryRun {
		t.Fatalf("status = %q", result.Status)
	}
}

func TestDryRunExecutorRejectsWritePermissions(t *testing.T) {
	_, err := (DryRunExecutor{}).Execute(context.Background(), WorkUnit{
		ID:      "work-01-backend",
		AgentID: "backend-engineer",
		Mode:    ModeReadOnly,
		Permissions: PermissionEnvelope{
			SourceCode: "write",
			Tests:      "read",
			GitCommit:  "deny",
			GitPush:    "deny",
			Secrets:    "deny",
			Production: "deny",
		},
	})
	if err == nil {
		t.Fatal("expected permission envelope error")
	}
}


func TestBuildWorkUnitsPreservesReadOnlyAgentAsNonMutating(t *testing.T) {
	plan := orchestration.Plan{
		Intent: orchestration.IntentReview,
		Agents: []orchestration.AgentPlan{{
			AgentID:      "reviewer",
			Capabilities: []string{"code-review"},
		}},
	}
	units, err := BuildWorkUnits("Review service", ".", plan, fixtureAgents())
	if err != nil {
		t.Fatal(err)
	}
	if len(units) != 1 {
		t.Fatalf("units = %#v", units)
	}
	if units[0].CanMutate() || units[0].Mutation.SourceCode || units[0].Mutation.Tests {
		t.Fatalf("reviewer mutation capability = %#v", units[0].Mutation)
	}
	if units[0].Mode != ModeReadOnly || units[0].Permissions.SourceCode != "read" || units[0].Permissions.Tests != "read" {
		t.Fatalf("effective unit = %#v", units[0])
	}
}


func TestFormatToolBindingsMarksMissingOperationEvidence(t *testing.T) {
	unit := WorkUnit{
		Tools: []string{"github"},
	}
	if got := FormatToolBindings(unit); got != "" {
		t.Fatalf("got=%q", got)
	}
}
