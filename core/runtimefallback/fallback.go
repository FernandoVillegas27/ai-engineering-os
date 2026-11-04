package runtimefallback

import (
	"context"
	"fmt"
	"strings"

	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/proposal"
	"github.com/FernandoVillegas27/ai-engineering-os/core/quality"
	"github.com/FernandoVillegas27/ai-engineering-os/core/review"
	"github.com/FernandoVillegas27/ai-engineering-os/core/staging"
)

type ErrorKind string

const (
	ErrorUnknown     ErrorKind = "unknown"
	ErrorUnavailable ErrorKind = "provider-unavailable"
)

type ProviderError struct {
	Provider string
	Kind     ErrorKind
	Err      error
}

func (e ProviderError) Error() string {
	if e.Err == nil {
		return e.Provider + ": provider error"
	}
	return e.Provider + ": " + e.Err.Error()
}

func (e ProviderError) Unwrap() error { return e.Err }

func Classify(provider string, err error) ProviderError {
	if err == nil {
		return ProviderError{Provider: provider, Kind: ErrorUnknown}
	}
	text := strings.ToLower(err.Error())
	unavailableMarkers := []string{
		"usage limit",
		"quota",
		"purchase more credits",
		"upgrade to pro",
		"not logged in",
		"login required",
		"authentication required",
		"unauthorized",
		"command not found in path",
		"could not resolve host",
		"network is unreachable",
		"connection timed out",
		"connection reset",
		"service unavailable",
		"temporarily unavailable",
	}
	for _, marker := range unavailableMarkers {
		if strings.Contains(text, marker) {
			return ProviderError{Provider: provider, Kind: ErrorUnavailable, Err: err}
		}
	}
	return ProviderError{Provider: provider, Kind: ErrorUnknown, Err: err}
}

func IsUnavailable(err error) bool {
	if err == nil {
		return false
	}
	var providerErr ProviderError
	if ok := asProviderError(err, &providerErr); ok {
		return providerErr.Kind == ErrorUnavailable
	}
	return Classify("", err).Kind == ErrorUnavailable
}

func asProviderError(err error, target *ProviderError) bool {
	for err != nil {
		if typed, ok := err.(ProviderError); ok {
			*target = typed
			return true
		}
		type unwrapper interface{ Unwrap() error }
		u, ok := err.(unwrapper)
		if !ok {
			break
		}
		err = u.Unwrap()
	}
	return false
}

type ExecutionRuntime struct {
	Primary  execution.Runtime
	Fallback execution.Runtime
}

func (r ExecutionRuntime) ID() string { return "execution-auto" }

func (r ExecutionRuntime) Analyze(ctx context.Context, unit execution.WorkUnit) (execution.Analysis, error) {
	if r.Primary == nil {
		return execution.Analysis{}, fmt.Errorf("primary execution runtime is required")
	}
	result, err := r.Primary.Analyze(ctx, unit)
	if err == nil {
		return result, nil
	}
	if r.Fallback == nil || !IsUnavailable(err) {
		return execution.Analysis{}, err
	}
	return r.Fallback.Analyze(ctx, unit)
}

type ProposalRuntime struct {
	Primary  proposal.Runtime
	Fallback proposal.Runtime
}

func (r ProposalRuntime) ID() string { return "proposal-auto" }

func (r ProposalRuntime) Propose(ctx context.Context, unit execution.WorkUnit) (proposal.Proposal, error) {
	if r.Primary == nil {
		return proposal.Proposal{}, fmt.Errorf("primary proposal runtime is required")
	}
	result, err := r.Primary.Propose(ctx, unit)
	if err == nil {
		return result, nil
	}
	if r.Fallback == nil || !IsUnavailable(err) {
		return proposal.Proposal{}, err
	}
	return r.Fallback.Propose(ctx, unit)
}

type StageRuntime struct {
	Primary  staging.Runtime
	Fallback staging.Runtime
}

func (r StageRuntime) ID() string { return "stage-auto" }

func (r StageRuntime) Stage(ctx context.Context, unit execution.WorkUnit, p proposal.Proposal) (staging.StageResult, error) {
	if r.Primary == nil {
		return staging.StageResult{}, fmt.Errorf("primary stage runtime is required")
	}
	result, err := r.Primary.Stage(ctx, unit, p)
	if err == nil {
		return result, nil
	}
	if r.Fallback == nil || !IsUnavailable(err) {
		return staging.StageResult{}, err
	}
	return r.Fallback.Stage(ctx, unit, p)
}

type ReviewRuntime struct {
	Primary  review.Runtime
	Fallback review.Runtime
}

func (r ReviewRuntime) ID() string { return "review-auto" }

func (r ReviewRuntime) Review(ctx context.Context, workspace, task string, changedFiles []string) (review.Report, error) {
	if r.Primary == nil {
		return review.Report{}, fmt.Errorf("primary review runtime is required")
	}
	result, err := r.Primary.Review(ctx, workspace, task, changedFiles)
	if err == nil {
		return result, nil
	}
	if r.Fallback == nil || !IsUnavailable(err) {
		return review.Report{}, err
	}
	return r.Fallback.Review(ctx, workspace, task, changedFiles)
}

type Corrector struct {
	Primary  quality.Corrector
	Fallback quality.Corrector
}

func (r Corrector) ID() string { return "correction-auto" }

func (r Corrector) Correct(ctx context.Context, request quality.CorrectionRequest) error {
	if r.Primary == nil {
		return fmt.Errorf("primary corrector is required")
	}
	err := r.Primary.Correct(ctx, request)
	if err == nil {
		return nil
	}
	if r.Fallback == nil || !IsUnavailable(err) {
		return err
	}
	return r.Fallback.Correct(ctx, request)
}
