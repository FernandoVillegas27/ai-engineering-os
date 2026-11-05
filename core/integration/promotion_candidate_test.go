package integration

import (
	"context"
	"testing"

	"github.com/FernandoVillegas27/ai-engineering-os/core/promotion"
	"github.com/FernandoVillegas27/ai-engineering-os/core/quality"
	"github.com/FernandoVillegas27/ai-engineering-os/core/taskworkspace"
)

func TestBuildPromotionCandidateUsesFinalQualityChanges(t *testing.T) {
	fixture := newMaterializeFixture(t)
	defer fixture.cleanup(t)
	integrated, err := Materialize(context.Background(), MaterializeConfig{
		OriginalRoot: fixture.original,
		Parent:       fixture.integrationParent,
	}, fixture.plan, []taskworkspace.Workspace{fixture.backend, fixture.frontend})
	if err != nil {
		t.Fatal(err)
	}
	defer Cleanup(integrated, fixture.integrationParent)

	gate := QualityGateResult{
		Promotable: true,
		Quality: quality.Result{
			Passed: true,
			Reason: quality.OutcomePassed,
		},
		FinalChanges: integrated.Changes,
	}
	candidate, err := BuildPromotionCandidate(
		fixture.original,
		integrated,
		gate,
		fixture.integrationParent,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !candidate.Request.QualityPassed {
		t.Fatal("candidate lost quality approval")
	}
	if candidate.Request.StagedWorkspace != integrated.Root ||
		candidate.Request.PreImage.Digest() != integrated.PreImage.Digest() {
		t.Fatalf("request = %#v", candidate.Request)
	}
	if len(candidate.Changes) != len(integrated.Changes) ||
		len(candidate.Request.Declared) != len(integrated.Changes) {
		t.Fatalf("candidate = %#v", candidate)
	}

	plan, err := promotion.DryRun(context.Background(), candidate.Request)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Ready || len(plan.Changes) != len(integrated.Changes) {
		t.Fatalf("promotion plan = %#v", plan)
	}
}

func TestBuildPromotionCandidateRejectsNonPromotableQuality(t *testing.T) {
	_, err := BuildPromotionCandidate(
		"/project",
		IntegratedWorkspace{},
		QualityGateResult{
			Promotable: false,
			Quality:    quality.Result{Passed: true},
		},
		"",
	)
	if err == nil {
		t.Fatal("expected non-promotable rejection")
	}

	_, err = BuildPromotionCandidate(
		"/project",
		IntegratedWorkspace{},
		QualityGateResult{
			Promotable: true,
			Quality:    quality.Result{Passed: false},
		},
		"",
	)
	if err == nil {
		t.Fatal("expected failed-quality rejection")
	}
}

func TestPromotionCandidateDoesNotApplyWithoutSeparateApproval(t *testing.T) {
	fixture := newMaterializeFixture(t)
	defer fixture.cleanup(t)
	integrated, err := Materialize(context.Background(), MaterializeConfig{
		OriginalRoot: fixture.original,
		Parent:       fixture.integrationParent,
	}, fixture.plan, []taskworkspace.Workspace{fixture.backend, fixture.frontend})
	if err != nil {
		t.Fatal(err)
	}
	defer Cleanup(integrated, fixture.integrationParent)

	candidate, err := BuildPromotionCandidate(
		fixture.original,
		integrated,
		QualityGateResult{
			Promotable:   true,
			Quality:      quality.Result{Passed: true, Reason: quality.OutcomePassed},
			FinalChanges: integrated.Changes,
		},
		fixture.integrationParent,
	)
	if err != nil {
		t.Fatal(err)
	}

	result, err := promotion.Apply(context.Background(), promotion.ApplyRequest{
		Request:  candidate.Request,
		Approved: false,
	})
	if err == nil {
		t.Fatal("expected explicit approval requirement")
	}
	if result.Applied {
		t.Fatalf("result = %#v", result)
	}
}
