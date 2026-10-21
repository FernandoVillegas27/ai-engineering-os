package quality

import (
	"context"
	"fmt"
	"testing"

	"github.com/FernandoVillegas27/ai-engineering-os/core/review"
	"github.com/FernandoVillegas27/ai-engineering-os/core/verification"
)

type fakeVerifier struct {
	reports []verification.Report
	calls   int
}

func (f *fakeVerifier) ID() string { return "fake-verifier" }

func (f *fakeVerifier) Verify(context.Context, string) (verification.Report, error) {
	index := f.calls
	f.calls++
	if index >= len(f.reports) {
		index = len(f.reports) - 1
	}
	return f.reports[index], nil
}

type fakeReviewer struct {
	reports []review.Report
	calls   int
}

func (f *fakeReviewer) ID() string { return "fake-reviewer" }

func (f *fakeReviewer) Review(context.Context, string, string, []string) (review.Report, error) {
	index := f.calls
	f.calls++
	if index >= len(f.reports) {
		index = len(f.reports) - 1
	}
	return f.reports[index], nil
}

type fakeCorrector struct {
	calls int
	err   error
}

func (f *fakeCorrector) ID() string { return "fake-corrector" }

func (f *fakeCorrector) Correct(context.Context, CorrectionRequest) error {
	f.calls++
	return f.err
}

func TestLoopPassesImmediately(t *testing.T) {
	verifier := &fakeVerifier{reports: []verification.Report{{Status: verification.StatusPassed}}}
	reviewer := &fakeReviewer{reports: []review.Report{{Verdict: review.VerdictApproved}}}
	corrector := &fakeCorrector{}

	result, err := (Loop{
		Verifier: verifier,
		Reviewer: reviewer,
		Corrector: corrector,
	}).Run(context.Background(), "workspace", "task", []string{"a.go"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Passed || len(result.Attempts) != 1 {
		t.Fatalf("result = %#v", result)
	}
	if corrector.calls != 0 {
		t.Fatalf("corrector calls = %d", corrector.calls)
	}
}

func TestLoopCorrectsThenPasses(t *testing.T) {
	verifier := &fakeVerifier{reports: []verification.Report{
		{Status: verification.StatusFailed},
		{Status: verification.StatusPassed},
	}}
	reviewer := &fakeReviewer{reports: []review.Report{
		{Verdict: review.VerdictApproved},
	}}
	corrector := &fakeCorrector{}

	result, err := (Loop{
		Verifier:    verifier,
		Reviewer:    reviewer,
		Corrector:   corrector,
		MaxAttempts: 3,
	}).Run(context.Background(), "workspace", "task", []string{"a.go"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Passed || len(result.Attempts) != 2 {
		t.Fatalf("result = %#v", result)
	}
	if !result.Attempts[0].Corrected {
		t.Fatal("first attempt should be corrected")
	}
	if corrector.calls != 1 {
		t.Fatalf("corrector calls = %d", corrector.calls)
	}
}

func TestLoopStopsAtMaxAttempts(t *testing.T) {
	verifier := &fakeVerifier{reports: []verification.Report{{Status: verification.StatusFailed}}}
	reviewer := &fakeReviewer{reports: []review.Report{{Verdict: review.VerdictRejected}}}
	corrector := &fakeCorrector{}

	result, err := (Loop{
		Verifier:    verifier,
		Reviewer:    reviewer,
		Corrector:   corrector,
		MaxAttempts: 2,
	}).Run(context.Background(), "workspace", "task", []string{"a.go"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Passed || len(result.Attempts) != 2 {
		t.Fatalf("result = %#v", result)
	}
	if corrector.calls != 1 {
		t.Fatalf("corrector calls = %d", corrector.calls)
	}
}


func TestLoopStopsWithoutCorrectionOnInfrastructureFailure(t *testing.T) {
	verifier := &fakeVerifier{reports: []verification.Report{{
		Status: verification.StatusFailed,
		Checks: []verification.Check{{
			Name:        "go-test",
			Passed:      false,
			FailureKind: verification.FailureInfrastructure,
			Correctable: false,
		}},
	}}}
	reviewer := &fakeReviewer{reports: []review.Report{{Verdict: review.VerdictApproved}}}
	corrector := &fakeCorrector{}

	result, err := (Loop{
		Verifier:    verifier,
		Reviewer:    reviewer,
		Corrector:   corrector,
		MaxAttempts: 3,
	}).Run(context.Background(), "workspace", "task", []string{"a.go"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Passed || len(result.Attempts) != 1 || !result.Attempts[0].Blocked {
		t.Fatalf("result = %#v", result)
	}
	if corrector.calls != 0 {
		t.Fatalf("corrector calls = %d", corrector.calls)
	}
}

func TestLoopPreservesCorrectionError(t *testing.T) {
	verifier := &fakeVerifier{reports: []verification.Report{{
		Status: verification.StatusFailed,
		Checks: []verification.Check{{
			Name:        "go-test",
			Passed:      false,
			FailureKind: verification.FailureCode,
			Correctable: true,
		}},
	}}}
	reviewer := &fakeReviewer{reports: []review.Report{{Verdict: review.VerdictApproved}}}
	corrector := &fakeCorrector{err: fmt.Errorf("no file changes")}

	result, err := (Loop{
		Verifier:    verifier,
		Reviewer:    reviewer,
		Corrector:   corrector,
		MaxAttempts: 3,
	}).Run(context.Background(), "workspace", "task", []string{"a.go"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Attempts) != 1 || result.Attempts[0].CorrectionError == "" {
		t.Fatalf("result = %#v", result)
	}
}
