package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/FernandoVillegas27/ai-engineering-os/core/orchestration"
	"github.com/FernandoVillegas27/ai-engineering-os/core/registry"
	"github.com/FernandoVillegas27/ai-engineering-os/core/toolavailability"
	githubintegration "github.com/FernandoVillegas27/ai-engineering-os/integrations/githubcli"
)

func githubPlan() orchestration.Plan {
	return orchestration.Plan{
		Intent: orchestration.IntentReview,
		Agents: []orchestration.AgentPlan{{
			AgentID: "reviewer",
			Tools: orchestration.ToolPlan{
				Recommended: []string{"github"},
			},
		}},
	}
}

func TestGitHubBindingExposesOnlyVerifiedReadOperations(t *testing.T) {
	tools := context7TestRegistry()
	githubTool := githubToolFixture()
	tools.Tools["github"] = githubTool

	bindings := runtimeToolBindings{
		Availability: collectToolAvailability(context.Background(), tools, nil),
		Context: map[string]string{},
	}
	got := bindGitHubReadOnly(
		context.Background(),
		tools,
		githubPlan(),
		t.TempDir(),
		bindings,
		func(context.Context, string) (githubintegration.Snapshot, error) {
			return githubintegration.Snapshot{
				Context: "GitHub repository evidence\n{}",
				BoundOperations: []string{"read-repository", "read-pull-requests"},
			}, nil
		},
	)

	evidence := got.Availability["github"]
	if evidence.State != toolavailability.StateAvailable || !evidence.RuntimeBound || !evidence.Healthy {
		t.Fatalf("evidence=%#v", evidence)
	}
	if joined := strings.Join(evidence.BoundOperations, ","); joined != "read-pull-requests,read-repository" {
		t.Fatalf("operations=%q", joined)
	}
	if err := toolavailability.ValidateSnapshot(tools, got.Availability); err != nil {
		t.Fatal(err)
	}
	if got.Context["github"] == "" {
		t.Fatal("expected GitHub evidence context")
	}
}

func TestGitHubBindingFailureRemovesOptionalTool(t *testing.T) {
	tools := context7TestRegistry()
	tools.Tools["github"] = githubToolFixture()
	bindings := runtimeToolBindings{
		Availability: collectToolAvailability(context.Background(), tools, nil),
		Context: map[string]string{},
	}
	got := bindGitHubReadOnly(
		context.Background(),
		tools,
		githubPlan(),
		t.TempDir(),
		bindings,
		func(context.Context, string) (githubintegration.Snapshot, error) {
			return githubintegration.Snapshot{}, errors.New("not authenticated")
		},
	)
	if evidence := got.Availability["github"]; evidence.State != toolavailability.StateUnavailable || evidence.RuntimeBound {
		t.Fatalf("evidence=%#v", evidence)
	}
	plan, err := orchestration.ApplyToolAvailability(githubPlan(), tools, got.Availability)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Agents[0].Tools.Recommended) != 0 {
		t.Fatalf("tools=%#v", plan.Agents[0].Tools)
	}
}

func githubToolFixture() registry.Tool {
	provider := "github"
	check := "github-connection-available"
	return registry.Tool{
		ID: "github",
		Description: "github",
		Interface: registry.ToolInterface{Type: "connector", Provider: &provider},
		Operations: []string{
			"read-repository", "read-issues", "read-pull-requests",
			"create-branch", "create-commit", "create-pull-request", "review-pull-request",
		},
		Access: []string{"read", "write", "network"},
		Risk: "high",
		RequiresApproval: true,
		ApprovalRequiredOperations: []string{
			"create-branch", "create-commit", "create-pull-request", "review-pull-request",
		},
		AvailabilityCheck: &check,
		Status: "enabled",
	}
}
