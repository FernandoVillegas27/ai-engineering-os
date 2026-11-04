package integration

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/FernandoVillegas27/ai-engineering-os/core/baseline"
	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/quality"
	"github.com/FernandoVillegas27/ai-engineering-os/core/review"
	"github.com/FernandoVillegas27/ai-engineering-os/core/staging"
	"github.com/FernandoVillegas27/ai-engineering-os/core/verification"
)

type QualityGateConfig struct {
	OriginalRoot string
	Plan         Plan
	Workspace    IntegratedWorkspace
	Task         string
	Verifier     verification.Runtime
	Reviewer     review.Runtime
	Corrector    quality.Corrector
	MaxAttempts  int
	TempParent   string
}

type QualityGateResult struct {
	Promotable   bool                      `json:"promotable"`
	Quality      quality.Result            `json:"quality"`
	FinalChanges []staging.WorkspaceChange `json:"final_changes"`
}

func RunQualityGate(ctx context.Context, config QualityGateConfig) (QualityGateResult, error) {
	if strings.TrimSpace(config.OriginalRoot) == "" {
		return QualityGateResult{}, errors.New("integration quality gate requires original root")
	}
	if strings.TrimSpace(config.Workspace.Root) == "" {
		return QualityGateResult{}, errors.New("integration quality gate requires workspace")
	}
	if config.Workspace.PreImage.IsZero() {
		return QualityGateResult{}, errors.New("integration quality gate requires workspace pre-image")
	}
	if config.Workspace.PreImage.Digest() != config.Plan.PreImageDigest {
		return QualityGateResult{}, errors.New("integration workspace pre-image does not match integration plan")
	}
	if config.Verifier == nil {
		return QualityGateResult{}, errors.New("integration quality gate requires verifier")
	}
	if config.Reviewer == nil {
		return QualityGateResult{}, errors.New("integration quality gate requires reviewer")
	}
	if strings.TrimSpace(config.Task) == "" {
		return QualityGateResult{}, errors.New("integration quality gate requires task")
	}

	declared := plannedWorkspaceChanges(config.Plan)
	if len(declared) == 0 {
		return QualityGateResult{}, errors.New("integration quality gate requires planned changes")
	}
	if err := exactChanges(config.Workspace.Changes, declared); err != nil {
		return QualityGateResult{}, fmt.Errorf("integration workspace does not match plan before quality: %w", err)
	}

	allowed := uniquePaths(declared)
	provider, err := baseline.New(baseline.Config{
		OriginalRoot: config.OriginalRoot,
		PreImage:     config.Workspace.PreImage,
		Authorized:   allowed,
		Verifier:     config.Verifier,
		TempParent:   config.TempParent,
	})
	if err != nil {
		return QualityGateResult{}, fmt.Errorf("prepare integration baseline: %w", err)
	}

	result, runErr := (quality.Loop{
		Verifier:         config.Verifier,
		Reviewer:         config.Reviewer,
		Corrector:        config.Corrector,
		BaselineProvider: provider,
		MaxAttempts:      config.MaxAttempts,
	}).Run(ctx, config.Workspace.Root, config.Task, allowed)

	finalChanges, inspectErr := staging.DetectWorkspaceChanges(
		config.Workspace.PreImage.Snapshot(),
		config.Workspace.Root,
		execution.DefaultSafeViewPolicy(),
	)
	if inspectErr != nil {
		return QualityGateResult{
			Quality: result,
		}, fmt.Errorf("inspect integration workspace after quality: %w", inspectErr)
	}
	if scopeErr := validateFinalScope(finalChanges, declared); scopeErr != nil {
		return QualityGateResult{
			Quality:      result,
			FinalChanges: finalChanges,
		}, fmt.Errorf("integration quality changed the authorized scope: %w", scopeErr)
	}

	gate := QualityGateResult{
		Promotable:   false,
		Quality:      result,
		FinalChanges: finalChanges,
	}
	if runErr != nil {
		return gate, runErr
	}
	if !result.Passed {
		return gate, nil
	}

	// Force a final original/staged integrity check even when the staged
	// verification passed cleanly and the lazy Quality Loop never needed a baseline.
	if _, err := provider.Baseline(ctx, config.Workspace.Root); err != nil {
		return gate, fmt.Errorf("final integration integrity check: %w", err)
	}

	// Re-inspect after the final integrity/baseline verification window. This
	// catches unexpected workspace mutation during that call as well.
	finalChanges, err = staging.DetectWorkspaceChanges(
		config.Workspace.PreImage.Snapshot(),
		config.Workspace.Root,
		execution.DefaultSafeViewPolicy(),
	)
	if err != nil {
		return gate, fmt.Errorf("final integration workspace inspection: %w", err)
	}
	if err := validateFinalScope(finalChanges, declared); err != nil {
		return gate, fmt.Errorf("final integration scope check: %w", err)
	}
	gate.FinalChanges = finalChanges
	gate.Promotable = true
	return gate, nil
}

func validateFinalScope(actual, declared []staging.WorkspaceChange) error {
	if len(actual) == 0 {
		return errors.New("integration workspace has no remaining changes")
	}
	expected := map[string]string{}
	for _, change := range declared {
		expected[change.Path] = change.Action
	}
	for _, change := range actual {
		action, ok := expected[change.Path]
		if !ok {
			return fmt.Errorf("unauthorized final path: %s", change.Path)
		}
		if action != change.Action {
			return fmt.Errorf(
				"final action mismatch for %s: actual=%s planned=%s",
				change.Path,
				change.Action,
				action,
			)
		}
	}
	return nil
}

func uniquePaths(changes []staging.WorkspaceChange) []string {
	seen := map[string]bool{}
	for _, change := range changes {
		seen[change.Path] = true
	}
	out := make([]string, 0, len(seen))
	for path := range seen {
		out = append(out, path)
	}
	sort.Strings(out)
	return out
}
