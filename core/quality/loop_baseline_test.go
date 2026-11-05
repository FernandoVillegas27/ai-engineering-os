package quality

import (
	"context"
	"errors"
	"testing"

	"github.com/FernandoVillegas27/ai-engineering-os/core/baseline"
	"github.com/FernandoVillegas27/ai-engineering-os/core/review"
	"github.com/FernandoVillegas27/ai-engineering-os/core/verification"
)

type fakeBaselineProvider struct {
	result baseline.Result
	err    error
	calls  int
}

func (f *fakeBaselineProvider) Baseline(context.Context, string) (baseline.Result, error) {
	f.calls++
	return f.result, f.err
}

type capturingCorrector struct {
	calls    int
	requests []CorrectionRequest
	err      error
}

func (c *capturingCorrector) ID() string { return "capturing-corrector" }

func (c *capturingCorrector) Correct(_ context.Context, request CorrectionRequest) error {
	c.calls++
	c.requests = append(c.requests, request)
	return c.err
}

func structuredFailureReport(failures ...verification.Failure) verification.Report {
	return verification.Report{
		Status: verification.StatusFailed,
		Checks: []verification.Check{{
			Name:         "go-test",
			Passed:       false,
			FailureKind:  verification.FailureCode,
			Correctable:  true,
			FailureParse: verification.ParseComplete,
			Failures:     append([]verification.Failure(nil), failures...),
			Stdout:       "full output that may contain several failures",
		}},
	}
}

func testFailure(id, signature string) verification.Failure {
	return verification.Failure{
		ID:        id,
		Class:     verification.FailureClassTest,
		Signature: signature,
	}
}

func TestLoopDoesNotRequestBaselineWhenVerificationPasses(t *testing.T) {
	verifier := &fakeVerifier{reports: []verification.Report{{Status: verification.StatusPassed}}}
	reviewer := &fakeReviewer{reports: []review.Report{{Verdict: review.VerdictApproved}}}
	provider := &fakeBaselineProvider{}

	result, err := (Loop{
		Verifier:         verifier,
		Reviewer:         reviewer,
		BaselineProvider: provider,
	}).Run(context.Background(), "workspace", "task", []string{"a.go"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Passed {
		t.Fatalf("result = %#v", result)
	}
	if provider.calls != 0 {
		t.Fatalf("baseline calls = %d, want 0", provider.calls)
	}
}

func TestLoopPreexistingOnlyRunsReviewWithoutCorrectionOrFalsePass(t *testing.T) {
	failure := testFailure("pkg#TestExisting", "same")
	current := structuredFailureReport(failure)
	verifier := &fakeVerifier{reports: []verification.Report{current}}
	reviewer := &fakeReviewer{reports: []review.Report{{Verdict: review.VerdictApproved}}}
	provider := &fakeBaselineProvider{result: baseline.Result{Report: structuredFailureReport(failure)}}
	corrector := &capturingCorrector{}

	result, err := (Loop{
		Verifier:         verifier,
		Reviewer:         reviewer,
		Corrector:        corrector,
		BaselineProvider: provider,
		MaxAttempts:      3,
	}).Run(context.Background(), "workspace", "task", []string{"a.go"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Passed {
		t.Fatal("pre-existing deterministic failures must never become PASS")
	}
	if len(result.Attempts) != 1 || !result.Attempts[0].BaselineCompared {
		t.Fatalf("result = %#v", result)
	}
	if result.Attempts[0].Comparison == nil || len(result.Attempts[0].Comparison.Preexisting) != 1 {
		t.Fatalf("comparison = %#v", result.Attempts[0].Comparison)
	}
	if reviewer.calls != 1 {
		t.Fatalf("reviewer calls = %d, want 1", reviewer.calls)
	}
	if corrector.calls != 0 {
		t.Fatalf("corrector calls = %d, want 0", corrector.calls)
	}
}

func TestLoopSendsOnlyRegressionFailureIdentityToCorrector(t *testing.T) {
	existing := testFailure("pkg#TestExisting", "existing")
	regression := testFailure("pkg#TestNew", "new")
	verifier := &fakeVerifier{reports: []verification.Report{structuredFailureReport(existing, regression)}}
	reviewer := &fakeReviewer{reports: []review.Report{{Verdict: review.VerdictApproved}}}
	provider := &fakeBaselineProvider{result: baseline.Result{Report: structuredFailureReport(existing)}}
	corrector := &capturingCorrector{err: errors.New("stop after capture")}

	result, err := (Loop{
		Verifier:         verifier,
		Reviewer:         reviewer,
		Corrector:        corrector,
		BaselineProvider: provider,
		MaxAttempts:      3,
	}).Run(context.Background(), "workspace", "task", []string{"a.go"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Attempts) != 1 || result.Attempts[0].CorrectionError == "" {
		t.Fatalf("result = %#v", result)
	}
	if corrector.calls != 1 || len(corrector.requests) != 1 {
		t.Fatalf("corrector calls = %d", corrector.calls)
	}
	got := corrector.requests[0].Verification
	if len(got.Checks) != 1 || len(got.Checks[0].Failures) != 1 {
		t.Fatalf("actionable report = %#v", got)
	}
	if got.Checks[0].Failures[0].ID != regression.ID {
		t.Fatalf("failure = %#v, want regression %q", got.Checks[0].Failures[0], regression.ID)
	}
	if got.Checks[0].Stdout != "" || got.Checks[0].Stderr != "" {
		t.Fatalf("structured actionable report leaked unfiltered output: %#v", got.Checks[0])
	}
	summary := FailureSummary(got, review.Report{})
	if summary != "verification go-test failed: pkg#TestNew (test)" {
		t.Fatalf("summary = %q", summary)
	}
}

func TestLoopBlocksCompileFailureEvenWhenPreexisting(t *testing.T) {
	build := verification.Failure{ID: "pkg", Class: verification.FailureClassBuild, Signature: "compile"}
	report := structuredFailureReport(build)
	verifier := &fakeVerifier{reports: []verification.Report{report}}
	reviewer := &fakeReviewer{}
	provider := &fakeBaselineProvider{result: baseline.Result{Report: report}}
	corrector := &capturingCorrector{}

	result, err := (Loop{
		Verifier:         verifier,
		Reviewer:         reviewer,
		Corrector:        corrector,
		BaselineProvider: provider,
	}).Run(context.Background(), "workspace", "task", []string{"a.go"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Attempts) != 1 || !result.Attempts[0].Blocked {
		t.Fatalf("result = %#v", result)
	}
	if corrector.calls != 0 {
		t.Fatalf("corrector calls = %d, want 0", corrector.calls)
	}
}

func TestLoopBlocksPartialStructuredVerificationWithoutBaselineOrCorrection(t *testing.T) {
	report := structuredFailureReport(testFailure("pkg#TestX", "x"))
	report.Checks[0].FailureParse = verification.ParsePartial
	verifier := &fakeVerifier{reports: []verification.Report{report}}
	reviewer := &fakeReviewer{}
	provider := &fakeBaselineProvider{}
	corrector := &capturingCorrector{}

	result, err := (Loop{
		Verifier:         verifier,
		Reviewer:         reviewer,
		Corrector:        corrector,
		BaselineProvider: provider,
	}).Run(context.Background(), "workspace", "task", []string{"a.go"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Attempts) != 1 || !result.Attempts[0].Blocked {
		t.Fatalf("result = %#v", result)
	}
	if provider.calls != 0 {
		t.Fatalf("baseline calls = %d, want 0", provider.calls)
	}
	if corrector.calls != 0 {
		t.Fatalf("corrector calls = %d, want 0", corrector.calls)
	}
}

func TestLoopBaselineErrorNeverCallsCorrector(t *testing.T) {
	report := structuredFailureReport(testFailure("pkg#TestX", "x"))
	verifier := &fakeVerifier{reports: []verification.Report{report}}
	reviewer := &fakeReviewer{}
	provider := &fakeBaselineProvider{err: &baseline.IntegrityError{Kind: baseline.KindOriginalChanged, Detail: "changed"}}
	corrector := &capturingCorrector{}

	_, err := (Loop{
		Verifier:         verifier,
		Reviewer:         reviewer,
		Corrector:        corrector,
		BaselineProvider: provider,
	}).Run(context.Background(), "workspace", "task", []string{"a.go"})
	if err == nil {
		t.Fatal("expected baseline error")
	}
	if corrector.calls != 0 {
		t.Fatalf("corrector calls = %d, want 0", corrector.calls)
	}
}

func TestLoopStopsOnRepeatedActionableFingerprint(t *testing.T) {
	regression := testFailure("pkg#TestNew", "same")
	current := structuredFailureReport(regression)
	verifier := &fakeVerifier{reports: []verification.Report{current, current}}
	reviewer := &fakeReviewer{}
	provider := &fakeBaselineProvider{result: baseline.Result{Report: verification.Report{Status: verification.StatusPassed}}}
	corrector := &capturingCorrector{}

	result, err := (Loop{
		Verifier:         verifier,
		Reviewer:         reviewer,
		Corrector:        corrector,
		BaselineProvider: provider,
		MaxAttempts:      3,
	}).Run(context.Background(), "workspace", "task", []string{"a.go"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Attempts) != 2 || !result.Attempts[1].NoProgress {
		t.Fatalf("result = %#v", result)
	}
	if corrector.calls != 1 {
		t.Fatalf("corrector calls = %d, want 1", corrector.calls)
	}
}
