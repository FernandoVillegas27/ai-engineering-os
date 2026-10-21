package quality

import (
	"context"
	"fmt"
	"strings"

	"github.com/FernandoVillegas27/ai-engineering-os/core/review"
	"github.com/FernandoVillegas27/ai-engineering-os/core/verification"
)

type CorrectionRequest struct {
	Workspace    string
	Task         string
	AllowedFiles []string
	Verification verification.Report
	Review       review.Report
}

type Corrector interface {
	ID() string
	Correct(ctx context.Context, request CorrectionRequest) error
}

type Attempt struct {
	Number          int
	Verification    verification.Report
	Review          review.Report
	Corrected       bool
	Blocked         bool
	CorrectionError string
}

type Result struct {
	Passed   bool
	Attempts []Attempt
}

type Loop struct {
	Verifier    verification.Runtime
	Reviewer    review.Runtime
	Corrector   Corrector
	MaxAttempts int
}

func (l Loop) Run(ctx context.Context, workspace, task string, allowedFiles []string) (Result, error) {
	if l.Verifier == nil {
		return Result{}, fmt.Errorf("verifier is required")
	}
	if l.Reviewer == nil {
		return Result{}, fmt.Errorf("reviewer is required")
	}
	maxAttempts := l.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 3
	}

	var result Result
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		verificationReport, err := l.Verifier.Verify(ctx, workspace)
		if err != nil {
			return result, fmt.Errorf("verification attempt %d: %w", attempt, err)
		}

		verificationOK := verificationReport.Status == verification.StatusPassed || verificationReport.Status == verification.StatusSkipped
		reviewReport := review.Report{
			Verdict: review.VerdictRejected,
			Summary: "Independent review deferred until deterministic verification passes.",
		}
		if verificationOK {
			reviewReport, err = l.Reviewer.Review(ctx, workspace, task, allowedFiles)
			if err != nil {
				return result, fmt.Errorf("review attempt %d: %w", attempt, err)
			}
		}

		entry := Attempt{
			Number:       attempt,
			Verification: verificationReport,
			Review:       reviewReport,
		}

		if verificationOK && reviewReport.Verdict == review.VerdictApproved {
			result.Attempts = append(result.Attempts, entry)
			result.Passed = true
			return result, nil
		}

		if attempt == maxAttempts {
			result.Attempts = append(result.Attempts, entry)
			return result, nil
		}
		if !verificationOK && !verificationCorrectable(verificationReport) {
			entry.Blocked = true
			result.Attempts = append(result.Attempts, entry)
			return result, nil
		}
		if l.Corrector == nil {
			result.Attempts = append(result.Attempts, entry)
			return result, fmt.Errorf("quality gate failed and no corrector is configured")
		}

		if err := l.Corrector.Correct(ctx, CorrectionRequest{
			Workspace:    workspace,
			Task:         task,
			AllowedFiles: append([]string{}, allowedFiles...),
			Verification: verificationReport,
			Review:       reviewReport,
		}); err != nil {
			entry.CorrectionError = err.Error()
			result.Attempts = append(result.Attempts, entry)
			return result, nil
		}
		entry.Corrected = true
		result.Attempts = append(result.Attempts, entry)
	}

	return result, nil
}

func FailureSummary(v verification.Report, r review.Report) string {
	var parts []string
	for _, check := range v.Checks {
		if !check.Passed {
			detail := strings.TrimSpace(check.Stderr)
			if detail == "" {
				detail = strings.TrimSpace(check.Stdout)
			}
			parts = append(parts, fmt.Sprintf("verification %s failed: %s", check.Name, detail))
		}
	}
	for _, finding := range r.Findings {
		if finding.Severity == "medium" || finding.Severity == "high" || finding.Severity == "critical" {
			parts = append(parts, fmt.Sprintf("review %s: %s - %s", finding.Severity, finding.Title, finding.Detail))
		}
	}
	return strings.Join(parts, "\n")
}


func verificationCorrectable(report verification.Report) bool {
	if report.Status != verification.StatusFailed {
		return true
	}
	foundFailure := false
	for _, check := range report.Checks {
		if check.Passed {
			continue
		}
		foundFailure = true
		if !check.Correctable {
			return false
		}
	}
	// Older/custom verifiers may not classify checks yet. Preserve the previous
	// behavior and allow one correction attempt rather than silently blocking.
	return !foundFailure || true
}
