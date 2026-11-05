package integration

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/FernandoVillegas27/ai-engineering-os/core/contractverify"
	"github.com/FernandoVillegas27/ai-engineering-os/core/quality"
	"github.com/FernandoVillegas27/ai-engineering-os/core/review"
	"github.com/FernandoVillegas27/ai-engineering-os/core/staging"
	"github.com/FernandoVillegas27/ai-engineering-os/core/taskworkspace"
	"github.com/FernandoVillegas27/ai-engineering-os/core/verification"
)

type sequenceVerifier struct {
	reports []verification.Report
	errs    []error
	calls   int
}

func (v *sequenceVerifier) ID() string { return "sequence-verifier" }

func (v *sequenceVerifier) Verify(context.Context, string) (verification.Report, error) {
	index := v.calls
	v.calls++
	if index < len(v.errs) && v.errs[index] != nil {
		return verification.Report{}, v.errs[index]
	}
	if index >= len(v.reports) {
		return verification.Report{Status: verification.StatusPassed}, nil
	}
	return v.reports[index], nil
}

type gateReviewer struct {
	report review.Report
	err    error
	hook   func()
	calls  int
}

func (r *gateReviewer) ID() string { return "gate-reviewer" }

func (r *gateReviewer) Review(context.Context, string, string, []string) (review.Report, error) {
	r.calls++
	if r.hook != nil {
		r.hook()
	}
	return r.report, r.err
}

type gateCorrector struct {
	hook  func() error
	calls int
}

func (c *gateCorrector) ID() string { return "gate-corrector" }

func (c *gateCorrector) Correct(context.Context, quality.CorrectionRequest) error {
	c.calls++
	if c.hook != nil {
		return c.hook()
	}
	return nil
}

func passReport() verification.Report {
	return verification.Report{Status: verification.StatusPassed}
}

func correctableFailureReport() verification.Report {
	return verification.Report{
		Status: verification.StatusFailed,
		Checks: []verification.Check{{
			Name:        "test",
			Passed:      false,
			FailureKind: verification.FailureCode,
			Correctable: true,
			Stderr:      "failure",
		}},
	}
}

func blockingFailureReport() verification.Report {
	return verification.Report{
		Status: verification.StatusFailed,
		Checks: []verification.Check{{
			Name:        "compile",
			Passed:      false,
			FailureKind: verification.FailureCode,
			Correctable: false,
			Stderr:      "compile failed",
		}},
	}
}

func TestRunQualityGateMarksCleanIntegratedWorkspacePromotable(t *testing.T) {
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

	verifier := &sequenceVerifier{reports: []verification.Report{
		passReport(),
		passReport(),
	}}
	reviewer := &gateReviewer{report: review.Report{Verdict: review.VerdictApproved, Summary: "approved"}}

	result, err := RunQualityGate(context.Background(), QualityGateConfig{
		OriginalRoot: fixture.original,
		Plan:         fixture.plan,
		Workspace:    integrated,
		Task:         "integrate backend and frontend change",
		Verifier:     verifier,
		Reviewer:     reviewer,
		MaxAttempts:  2,
		TempParent:   fixture.integrationParent,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Promotable || !result.Quality.Passed {
		t.Fatalf("result = %#v", result)
	}
	if len(result.FinalChanges) != len(fixture.plan.Changes) {
		t.Fatalf("final changes = %#v", result.FinalChanges)
	}
	if verifier.calls != 2 || reviewer.calls != 1 {
		t.Fatalf("verifier=%d reviewer=%d", verifier.calls, reviewer.calls)
	}
}

func TestRunQualityGateBlocksDeterministicFailure(t *testing.T) {
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

	result, err := RunQualityGate(context.Background(), QualityGateConfig{
		OriginalRoot: fixture.original,
		Plan:         fixture.plan,
		Workspace:    integrated,
		Task:         "integrate",
		Verifier:     &sequenceVerifier{reports: []verification.Report{blockingFailureReport()}},
		Reviewer:     &gateReviewer{report: review.Report{Verdict: review.VerdictApproved}},
		MaxAttempts:  2,
		TempParent:   fixture.integrationParent,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Promotable || result.Quality.Passed {
		t.Fatalf("result = %#v", result)
	}
}

func TestRunQualityGateRejectsCorrectorEscapingAuthorizedScope(t *testing.T) {
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

	corrector := &gateCorrector{hook: func() error {
		return os.WriteFile(filepath.Join(integrated.Root, "unauthorized.go"), []byte("package main\n"), 0o644)
	}}
	verifier := &sequenceVerifier{reports: []verification.Report{
		correctableFailureReport(),
		passReport(),
	}}
	reviewer := &gateReviewer{report: review.Report{Verdict: review.VerdictApproved}}

	result, err := RunQualityGate(context.Background(), QualityGateConfig{
		OriginalRoot: fixture.original,
		Plan:         fixture.plan,
		Workspace:    integrated,
		Task:         "integrate",
		Verifier:     verifier,
		Reviewer:     reviewer,
		Corrector:    corrector,
		MaxAttempts:  3,
		TempParent:   fixture.integrationParent,
	})
	if err == nil {
		t.Fatal("expected unauthorized final scope error")
	}
	if result.Promotable {
		t.Fatalf("result = %#v", result)
	}
	if corrector.calls != 1 {
		t.Fatalf("corrector calls=%d", corrector.calls)
	}
}

func TestRunQualityGateRejectsOriginalMutationDuringReview(t *testing.T) {
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

	reviewer := &gateReviewer{
		report: review.Report{Verdict: review.VerdictApproved},
		hook: func() {
			_ = os.WriteFile(filepath.Join(fixture.original, "main.go"), []byte("package main\nconst changed = true\n"), 0o644)
		},
	}
	result, err := RunQualityGate(context.Background(), QualityGateConfig{
		OriginalRoot: fixture.original,
		Plan:         fixture.plan,
		Workspace:    integrated,
		Task:         "integrate",
		Verifier:     &sequenceVerifier{reports: []verification.Report{passReport()}},
		Reviewer:     reviewer,
		MaxAttempts:  2,
		TempParent:   fixture.integrationParent,
	})
	if err == nil {
		t.Fatal("expected final integrity error")
	}
	if result.Promotable {
		t.Fatalf("result = %#v", result)
	}
}

func TestRunQualityGatePropagatesVerifierInfrastructureError(t *testing.T) {
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

	sentinel := errors.New("verifier unavailable")
	result, err := RunQualityGate(context.Background(), QualityGateConfig{
		OriginalRoot: fixture.original,
		Plan:         fixture.plan,
		Workspace:    integrated,
		Task:         "integrate",
		Verifier:     &sequenceVerifier{errs: []error{sentinel}},
		Reviewer:     &gateReviewer{report: review.Report{Verdict: review.VerdictApproved}},
		MaxAttempts:  2,
		TempParent:   fixture.integrationParent,
	})
	if err == nil || !errors.Is(err, sentinel) {
		t.Fatalf("err=%#v", err)
	}
	if result.Promotable {
		t.Fatalf("result = %#v", result)
	}
}


func TestRunQualityGateAcceptsExplicitFinalExpectedChanges(t *testing.T) {
	fixture := newMaterializeFixture(t)
	defer fixture.cleanup(t)
	base, err := Materialize(context.Background(), MaterializeConfig{
		OriginalRoot: fixture.original,
		Parent:       fixture.integrationParent,
	}, fixture.plan, []taskworkspace.Workspace{fixture.backend, fixture.frontend})
	if err != nil {
		t.Fatal(err)
	}
	defer Cleanup(base, fixture.integrationParent)

	qaParent := filepath.Join(fixture.base, "quality-qa")
	if err := os.MkdirAll(qaParent, 0o755); err != nil {
		t.Fatal(err)
	}
	manager, err := taskworkspace.New(taskworkspace.Config{
		OriginalRoot: base.Root,
		Parent:       qaParent,
	})
	if err != nil {
		t.Fatal(err)
	}
	qa, err := manager.Prepare(context.Background(), "qa", "qa-engineer", []staging.WorkspaceChange{
		{Path: "backend/service_test.go", Action: "create"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Cleanup(qa)
	writeIntegrationFile(t, qa.Root, "backend/service_test.go", "package backend\n")
	inspection, err := manager.Inspect(context.Background(), qa)
	if err != nil {
		t.Fatal(err)
	}
	final, err := OverlayTests(
		context.Background(),
		TestOverlayConfig{Parent: fixture.integrationParent},
		base,
		qa,
		inspection,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer Cleanup(final, fixture.integrationParent)

	verifier := &sequenceVerifier{reports: []verification.Report{passReport(), passReport()}}
	reviewer := &gateReviewer{report: review.Report{Verdict: review.VerdictApproved}}
	result, err := RunQualityGate(context.Background(), QualityGateConfig{
		OriginalRoot:    fixture.original,
		Plan:            fixture.plan,
		Workspace:       final,
		ExpectedChanges: final.Changes,
		Task:            "integrate source and QA tests",
		Verifier:        verifier,
		Reviewer:        reviewer,
		MaxAttempts:     2,
		TempParent:      fixture.integrationParent,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Promotable || len(result.FinalChanges) != len(final.Changes) {
		t.Fatalf("result=%#v", result)
	}
}


func TestRunQualityGateBlocksContractOrArchitectureRegression(t *testing.T) {
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

	verifier := &sequenceVerifier{reports: []verification.Report{
		passReport(),
		passReport(),
	}}
	reviewer := &gateReviewer{report: review.Report{Verdict: review.VerdictApproved}}

	result, err := RunQualityGate(context.Background(), QualityGateConfig{
		OriginalRoot: fixture.original,
		Plan:         fixture.plan,
		Workspace:    integrated,
		Task:         "integrate",
		Verifier:     verifier,
		Reviewer:     reviewer,
		MaxAttempts:  2,
		TempParent:   fixture.integrationParent,
		ContractVerifier: func(string, string) (contractverify.Result, error) {
			return contractverify.Result{
				Passed: false,
				Findings: []contractverify.Finding{{
					Kind: contractverify.FindingOpenAPI,
					ID: "operation-removed:get:/users",
					Path: "openapi.yaml",
					Summary: "GET /users was removed",
				}},
			}, nil
		},
	})
	if err == nil {
		t.Fatal("expected contract/architecture gate error")
	}
	if result.Promotable || !result.Quality.Passed {
		t.Fatalf("result=%#v", result)
	}
	if result.Contracts.Passed || len(result.Contracts.Findings) != 1 {
		t.Fatalf("contracts=%#v", result.Contracts)
	}
}

func TestRunQualityGateRecordsPassingContractVerification(t *testing.T) {
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

	result, err := RunQualityGate(context.Background(), QualityGateConfig{
		OriginalRoot: fixture.original,
		Plan:         fixture.plan,
		Workspace:    integrated,
		Task:         "integrate",
		Verifier:     &sequenceVerifier{reports: []verification.Report{passReport(), passReport()}},
		Reviewer:     &gateReviewer{report: review.Report{Verdict: review.VerdictApproved}},
		MaxAttempts:  2,
		TempParent:   fixture.integrationParent,
		ContractVerifier: func(string, string) (contractverify.Result, error) {
			return contractverify.Result{Passed: true}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Promotable || !result.Contracts.Passed {
		t.Fatalf("result=%#v", result)
	}
}
