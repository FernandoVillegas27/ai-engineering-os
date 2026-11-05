package quality

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/FernandoVillegas27/ai-engineering-os/core/baseline"
	"github.com/FernandoVillegas27/ai-engineering-os/core/review"
	"github.com/FernandoVillegas27/ai-engineering-os/core/verification"
)

type OutcomeReason string

const (
	OutcomePassed                 OutcomeReason = "passed"
	OutcomePreexistingOnly        OutcomeReason = "preexisting-only"
	OutcomeVerificationRegression OutcomeReason = "verification-regression"
	OutcomeReviewRejected         OutcomeReason = "review-rejected"
	OutcomeIntegrityFailure       OutcomeReason = "integrity-failure"
	OutcomeInfrastructureFailure  OutcomeReason = "infrastructure-failure"
	OutcomePartialVerification    OutcomeReason = "partial-verification"
	OutcomeNoProgress             OutcomeReason = "no-progress"
	OutcomeMaxAttempts            OutcomeReason = "max-attempts"
	OutcomeCancelled              OutcomeReason = "cancelled"
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

type BaselineProvider interface {
	Baseline(ctx context.Context, stagedWorkspace string) (baseline.Result, error)
}

type Attempt struct {
	Number                int
	Verification          verification.Report
	Review                review.Report
	Reviewed              bool
	Corrected             bool
	CorrectionAttempted   bool
	Blocked               bool
	NoProgress            bool
	BaselineCompared      bool
	Comparison            *Comparison
	ActionableFingerprint string
	CorrectionError       string
}

type ResultSummary struct {
	VerificationAttempts int
	ReviewAttempts       int
	CorrectionAttempts   int
	BaselineComparisons  int
}

type Result struct {
	Passed   bool
	Reason   OutcomeReason
	Attempts []Attempt
	Summary  ResultSummary
}

type Loop struct {
	Verifier         verification.Runtime
	Reviewer         review.Runtime
	Corrector        Corrector
	BaselineProvider BaselineProvider
	MaxAttempts      int
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
	var previousCorrectionFingerprint string

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			result.Reason = OutcomeCancelled
			return result, fmt.Errorf("quality loop canceled: %w", err)
		}

		verificationReport, err := l.Verifier.Verify(ctx, workspace)
		if err != nil {
			if ctx.Err() != nil {
				result.Reason = OutcomeCancelled
			} else {
				result.Reason = OutcomeInfrastructureFailure
			}
			return result, fmt.Errorf("verification attempt %d: %w", attempt, err)
		}

		verificationOK := verificationReport.Status == verification.StatusPassed || verificationReport.Status == verification.StatusSkipped
		reviewReport := review.Report{
			Verdict: review.VerdictRejected,
			Summary: "Independent review deferred until deterministic verification passes.",
		}
		reviewed := false
		if verificationOK {
			reviewReport, err = l.Reviewer.Review(ctx, workspace, task, allowedFiles)
			if err != nil {
				if ctx.Err() != nil {
					result.Reason = OutcomeCancelled
				} else {
					result.Reason = OutcomeInfrastructureFailure
				}
				return result, fmt.Errorf("review attempt %d: %w", attempt, err)
			}
			reviewed = true
		}

		entry := Attempt{
			Number:       attempt,
			Verification: verificationReport,
			Review:       reviewReport,
			Reviewed:     reviewed,
		}

		if verificationOK && reviewReport.Verdict == review.VerdictApproved {
			result.Passed = true
			result.Reason = OutcomePassed
			appendAttempt(&result, entry)
			return result, nil
		}

		if !verificationOK && !verificationCorrectable(verificationReport) {
			entry.Blocked = true
			if hasInfrastructureFailure(verificationReport) {
				result.Reason = OutcomeInfrastructureFailure
			} else {
				result.Reason = OutcomeVerificationRegression
			}
			appendAttempt(&result, entry)
			return result, nil
		}

		actionableVerification := verificationReport
		preexistingOnly := false

		if !verificationOK && l.BaselineProvider != nil {
			if unsafeStructuredReport(verificationReport) {
				entry.Blocked = true
				result.Reason = OutcomePartialVerification
				appendAttempt(&result, entry)
				return result, nil
			}

			if hasStructuredFailures(verificationReport) {
				baselineResult, baselineErr := l.BaselineProvider.Baseline(ctx, workspace)
				if baselineErr != nil {
					entry.Blocked = true
					if errors.Is(baselineErr, baseline.ErrIntegrity) {
						result.Reason = OutcomeIntegrityFailure
					} else if ctx.Err() != nil {
						result.Reason = OutcomeCancelled
					} else {
						result.Reason = OutcomeInfrastructureFailure
					}
					appendAttempt(&result, entry)
					return result, fmt.Errorf("baseline attempt %d: %w", attempt, baselineErr)
				}
				if hasInfrastructureFailure(baselineResult.Report) {
					entry.Blocked = true
					result.Reason = OutcomeInfrastructureFailure
					appendAttempt(&result, entry)
					return result, nil
				}
				if unsafeStructuredReport(baselineResult.Report) {
					entry.Blocked = true
					result.Reason = OutcomePartialVerification
					appendAttempt(&result, entry)
					return result, nil
				}

				comparison := Compare(baselineResult.Report, verificationReport)
				entry.BaselineCompared = true
				entry.Comparison = &comparison

				if len(comparison.Blocking) > 0 {
					entry.Blocked = true
					result.Reason = OutcomeVerificationRegression
					appendAttempt(&result, entry)
					return result, nil
				}

				actionableVerification = actionableReport(verificationReport, comparison)
				preexistingOnly = len(comparison.Preexisting) > 0 &&
					len(comparison.Regressions) == 0 &&
					len(comparison.ChangedCause) == 0 &&
					len(comparison.Unclassified) == 0

				if preexistingOnly {
					reviewReport, err = l.Reviewer.Review(ctx, workspace, task, allowedFiles)
					if err != nil {
						if ctx.Err() != nil {
							result.Reason = OutcomeCancelled
						} else {
							result.Reason = OutcomeInfrastructureFailure
						}
						return result, fmt.Errorf("review attempt %d: %w", attempt, err)
					}
					entry.Review = reviewReport
					entry.Reviewed = true
					if reviewReport.Verdict == review.VerdictApproved {
						result.Reason = OutcomePreexistingOnly
						appendAttempt(&result, entry)
						// The staged change introduced no known regression, but the
						// deterministic verification still fails. Never turn that
						// into a false PASS.
						return result, nil
					}
				}
			}
		}

		if attempt == maxAttempts {
			result.Reason = OutcomeMaxAttempts
			appendAttempt(&result, entry)
			return result, nil
		}
		if l.Corrector == nil {
			result.Reason = failureOutcome(verificationOK, reviewReport)
			appendAttempt(&result, entry)
			return result, fmt.Errorf("quality gate failed and no corrector is configured")
		}

		request := CorrectionRequest{
			Workspace:    workspace,
			Task:         task,
			AllowedFiles: append([]string{}, allowedFiles...),
			Verification: actionableVerification,
			Review:       reviewReport,
		}
		fingerprint := correctionFingerprint(request)
		entry.ActionableFingerprint = fingerprint
		if fingerprint != "" && fingerprint == previousCorrectionFingerprint {
			entry.NoProgress = true
			result.Reason = OutcomeNoProgress
			appendAttempt(&result, entry)
			return result, nil
		}

		entry.CorrectionAttempted = true
		if err := l.Corrector.Correct(ctx, request); err != nil {
			entry.CorrectionError = err.Error()
			result.Reason = failureOutcome(verificationOK || preexistingOnly, reviewReport)
			appendAttempt(&result, entry)
			return result, nil
		}
		previousCorrectionFingerprint = fingerprint
		entry.Corrected = true
		appendAttempt(&result, entry)
	}

	result.Reason = OutcomeMaxAttempts
	return result, nil
}

func appendAttempt(result *Result, entry Attempt) {
	result.Attempts = append(result.Attempts, entry)
	result.Summary.VerificationAttempts++
	if entry.Reviewed {
		result.Summary.ReviewAttempts++
	}
	if entry.CorrectionAttempted {
		result.Summary.CorrectionAttempts++
	}
	if entry.BaselineCompared {
		result.Summary.BaselineComparisons++
	}
}

func failureOutcome(verificationSatisfied bool, reviewReport review.Report) OutcomeReason {
	if verificationSatisfied && reviewReport.Verdict == review.VerdictRejected {
		return OutcomeReviewRejected
	}
	return OutcomeVerificationRegression
}

func FailureSummary(v verification.Report, r review.Report) string {
	var parts []string
	for _, check := range v.Checks {
		if check.Passed {
			continue
		}
		if len(check.Failures) > 0 {
			for _, failure := range sortedFailures(check.Failures) {
				parts = append(parts, fmt.Sprintf("verification %s failed: %s (%s)", check.Name, failure.ID, failure.Class))
			}
			continue
		}
		detail := strings.TrimSpace(check.Stderr)
		if detail == "" {
			detail = strings.TrimSpace(check.Stdout)
		}
		parts = append(parts, fmt.Sprintf("verification %s failed: %s", check.Name, detail))
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
	for _, check := range report.Checks {
		if check.Passed {
			continue
		}
		if !check.Correctable {
			return false
		}
	}
	// Older/custom verifiers may not classify checks yet. Preserve the previous
	// behavior and allow one correction attempt rather than silently blocking.
	return true
}

func hasInfrastructureFailure(report verification.Report) bool {
	for _, check := range report.Checks {
		if !check.Passed && check.FailureKind == verification.FailureInfrastructure {
			return true
		}
	}
	return false
}

func hasStructuredFailures(report verification.Report) bool {
	for _, check := range report.Checks {
		if !check.Passed && check.FailureParse != verification.ParseNotAttempted {
			return true
		}
	}
	return false
}

func unsafeStructuredReport(report verification.Report) bool {
	for _, check := range report.Checks {
		if check.Passed {
			continue
		}
		switch check.FailureParse {
		case verification.ParsePartial, verification.ParseFailed:
			return true
		case verification.ParseComplete:
			if len(check.Failures) == 0 {
				return true
			}
			seen := map[string]bool{}
			for _, failure := range check.Failures {
				if strings.TrimSpace(failure.ID) == "" || seen[failure.ID] {
					return true
				}
				seen[failure.ID] = true
			}
		}
	}
	return false
}

func actionableReport(after verification.Report, comparison Comparison) verification.Report {
	actionableIDs := map[string]map[string]bool{}
	unclassifiedChecks := map[string]bool{}

	add := func(check string, failure verification.Failure) {
		if actionableIDs[check] == nil {
			actionableIDs[check] = map[string]bool{}
		}
		actionableIDs[check][failure.ID] = true
	}
	for _, failure := range comparison.Regressions {
		add(failure.Check, failure.Failure)
	}
	for _, failure := range comparison.ChangedCause {
		add(failure.Check, failure.After)
	}
	for _, item := range comparison.Unclassified {
		unclassifiedChecks[item.Check] = true
	}

	result := verification.Report{Status: verification.StatusPassed}
	for _, check := range after.Checks {
		if check.Passed {
			continue
		}
		if unclassifiedChecks[check.Name] {
			result.Checks = append(result.Checks, cloneCheck(check))
			result.Status = verification.StatusFailed
			continue
		}
		ids := actionableIDs[check.Name]
		if len(ids) == 0 {
			continue
		}
		filtered := cloneCheck(check)
		filtered.Failures = filtered.Failures[:0]
		for _, failure := range check.Failures {
			if ids[failure.ID] {
				filtered.Failures = append(filtered.Failures, failure)
			}
		}
		// Structured summaries use only the filtered failure identities, so
		// pre-existing failure output is not forwarded to the Corrector.
		filtered.Stdout = ""
		filtered.Stderr = ""
		result.Checks = append(result.Checks, filtered)
		result.Status = verification.StatusFailed
	}
	return result
}

func cloneCheck(check verification.Check) verification.Check {
	cloned := check
	cloned.Command = append([]string(nil), check.Command...)
	cloned.Failures = append([]verification.Failure(nil), check.Failures...)
	return cloned
}

func correctionFingerprint(request CorrectionRequest) string {
	var items []string
	for _, check := range request.Verification.Checks {
		if check.Passed {
			continue
		}
		if len(check.Failures) > 0 {
			for _, failure := range check.Failures {
				items = append(items, fmt.Sprintf("v|%s|%s|%s|%s", check.Name, failure.ID, failure.Class, failure.Signature))
			}
		} else {
			items = append(items, fmt.Sprintf("v|%s|%s|%s|%s", check.Name, check.FailureKind, check.Stdout, check.Stderr))
		}
	}
	for _, finding := range request.Review.Findings {
		if finding.Severity == "medium" || finding.Severity == "high" || finding.Severity == "critical" {
			items = append(items, fmt.Sprintf("r|%s|%s|%s", finding.Severity, finding.Title, finding.Detail))
		}
	}
	if len(items) == 0 && request.Review.Verdict == review.VerdictRejected && strings.TrimSpace(request.Review.Summary) != "" {
		items = append(items, "r|rejected|"+request.Review.Summary)
	}
	if len(items) == 0 {
		return ""
	}
	sort.Strings(items)
	sum := sha256.Sum256([]byte(strings.Join(items, "\n")))
	return fmt.Sprintf("%x", sum[:])
}
