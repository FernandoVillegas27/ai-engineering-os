package quality

import (
	"context"
	"testing"

	"github.com/FernandoVillegas27/ai-engineering-os/core/baseline"
	"github.com/FernandoVillegas27/ai-engineering-os/core/review"
	"github.com/FernandoVillegas27/ai-engineering-os/core/verification"
)

func TestOutcomePassedIncludesAttemptSummary(t *testing.T) {
	verifier := &fakeVerifier{reports: []verification.Report{{Status: verification.StatusPassed}}}
	reviewer := &fakeReviewer{reports: []review.Report{{Verdict: review.VerdictApproved}}}

	result, err := (Loop{Verifier: verifier, Reviewer: reviewer}).Run(
		context.Background(), "workspace", "task", []string{"a.go"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Reason != OutcomePassed || !result.Passed {
		t.Fatalf("result = %#v", result)
	}
	if result.Summary.VerificationAttempts != 1 || result.Summary.ReviewAttempts != 1 {
		t.Fatalf("summary = %#v", result.Summary)
	}
	if result.Summary.CorrectionAttempts != 0 || result.Summary.BaselineComparisons != 0 {
		t.Fatalf("summary = %#v", result.Summary)
	}
}

func TestOutcomePreexistingOnlyIsExplicit(t *testing.T) {
	failure := testFailure("pkg#TestExisting", "same")
	current := structuredFailureReport(failure)
	verifier := &fakeVerifier{reports: []verification.Report{current}}
	reviewer := &fakeReviewer{reports: []review.Report{{Verdict: review.VerdictApproved}}}
	provider := &fakeBaselineProvider{result: baseline.Result{Report: structuredFailureReport(failure)}}

	result, err := (Loop{
		Verifier:         verifier,
		Reviewer:         reviewer,
		BaselineProvider: provider,
	}).Run(context.Background(), "workspace", "task", []string{"a.go"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Reason != OutcomePreexistingOnly || result.Passed {
		t.Fatalf("result = %#v", result)
	}
	if result.Summary.BaselineComparisons != 1 || result.Summary.ReviewAttempts != 1 {
		t.Fatalf("summary = %#v", result.Summary)
	}
}

func TestOutcomeIntegrityFailureIsExplicit(t *testing.T) {
	report := structuredFailureReport(testFailure("pkg#TestX", "x"))
	verifier := &fakeVerifier{reports: []verification.Report{report}}
	reviewer := &fakeReviewer{}
	provider := &fakeBaselineProvider{err: &baseline.IntegrityError{
		Kind:   baseline.KindOriginalChanged,
		Detail: "changed",
	}}

	result, err := (Loop{
		Verifier:         verifier,
		Reviewer:         reviewer,
		BaselineProvider: provider,
	}).Run(context.Background(), "workspace", "task", []string{"a.go"})
	if err == nil {
		t.Fatal("expected integrity error")
	}
	if result.Reason != OutcomeIntegrityFailure {
		t.Fatalf("reason = %q", result.Reason)
	}
	if len(result.Attempts) != 1 || !result.Attempts[0].Blocked {
		t.Fatalf("result = %#v", result)
	}
}

func TestOutcomeInfrastructureFailureIsExplicit(t *testing.T) {
	report := verification.Report{
		Status: verification.StatusFailed,
		Checks: []verification.Check{{
			Name:        "go-test",
			Passed:      false,
			FailureKind: verification.FailureInfrastructure,
			Correctable: false,
		}},
	}
	result, err := (Loop{
		Verifier: &fakeVerifier{reports: []verification.Report{report}},
		Reviewer: &fakeReviewer{},
	}).Run(context.Background(), "workspace", "task", []string{"a.go"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Reason != OutcomeInfrastructureFailure {
		t.Fatalf("result = %#v", result)
	}
}

func TestOutcomePartialVerificationIsExplicit(t *testing.T) {
	report := structuredFailureReport(testFailure("pkg#TestX", "x"))
	report.Checks[0].FailureParse = verification.ParsePartial

	result, err := (Loop{
		Verifier:         &fakeVerifier{reports: []verification.Report{report}},
		Reviewer:         &fakeReviewer{},
		BaselineProvider: &fakeBaselineProvider{},
	}).Run(context.Background(), "workspace", "task", []string{"a.go"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Reason != OutcomePartialVerification {
		t.Fatalf("result = %#v", result)
	}
}

func TestOutcomeNoProgressCountsOnlyActualCorrectionCall(t *testing.T) {
	regression := testFailure("pkg#TestNew", "same")
	current := structuredFailureReport(regression)
	verifier := &fakeVerifier{reports: []verification.Report{current, current}}
	provider := &fakeBaselineProvider{result: baseline.Result{Report: verification.Report{Status: verification.StatusPassed}}}
	corrector := &capturingCorrector{}

	result, err := (Loop{
		Verifier:         verifier,
		Reviewer:         &fakeReviewer{},
		Corrector:        corrector,
		BaselineProvider: provider,
		MaxAttempts:      3,
	}).Run(context.Background(), "workspace", "task", []string{"a.go"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Reason != OutcomeNoProgress {
		t.Fatalf("result = %#v", result)
	}
	if result.Summary.CorrectionAttempts != 1 {
		t.Fatalf("summary = %#v", result.Summary)
	}
	if result.Summary.BaselineComparisons != 2 || result.Summary.VerificationAttempts != 2 {
		t.Fatalf("summary = %#v", result.Summary)
	}
}

func TestOutcomeMaxAttemptsIsExplicit(t *testing.T) {
	report := verification.Report{Status: verification.StatusFailed}
	result, err := (Loop{
		Verifier:    &fakeVerifier{reports: []verification.Report{report}},
		Reviewer:    &fakeReviewer{},
		Corrector:   &capturingCorrector{},
		MaxAttempts: 2,
	}).Run(context.Background(), "workspace", "task", []string{"a.go"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Reason != OutcomeMaxAttempts {
		t.Fatalf("result = %#v", result)
	}
	if result.Summary.VerificationAttempts != 2 || result.Summary.CorrectionAttempts != 1 {
		t.Fatalf("summary = %#v", result.Summary)
	}
}
