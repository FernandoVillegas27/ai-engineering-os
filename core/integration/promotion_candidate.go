package integration

import (
	"errors"
	"strings"

	"github.com/FernandoVillegas27/ai-engineering-os/core/promotion"
	"github.com/FernandoVillegas27/ai-engineering-os/core/staging"
)

type PromotionCandidate struct {
	Request promotion.Request
	Changes []staging.WorkspaceChange
}

func BuildPromotionCandidate(
	originalRoot string,
	workspace IntegratedWorkspace,
	gate QualityGateResult,
	tempParent string,
) (PromotionCandidate, error) {
	if !gate.Promotable || !gate.Quality.Passed {
		return PromotionCandidate{}, errors.New("integration result is not promotable")
	}
	if strings.TrimSpace(originalRoot) == "" {
		return PromotionCandidate{}, errors.New("promotion candidate requires original root")
	}
	if strings.TrimSpace(workspace.Root) == "" || workspace.PreImage.IsZero() {
		return PromotionCandidate{}, errors.New("promotion candidate requires integrated workspace and pre-image")
	}
	if len(gate.FinalChanges) == 0 {
		return PromotionCandidate{}, errors.New("promotion candidate requires final changes")
	}

	declared := append([]staging.WorkspaceChange(nil), gate.FinalChanges...)
	return PromotionCandidate{
		Request: promotion.Request{
			OriginalRoot:    originalRoot,
			StagedWorkspace: workspace.Root,
			PreImage:        workspace.PreImage,
			Declared:        declared,
			QualityPassed:   true,
			TempParent:      tempParent,
		},
		Changes: declared,
	}, nil
}
