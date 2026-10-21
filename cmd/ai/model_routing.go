package main

import (
	"context"
	"fmt"

	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/modelpolicy"
	"github.com/FernandoVillegas27/ai-engineering-os/core/proposal"
	"github.com/FernandoVillegas27/ai-engineering-os/core/quality"
	"github.com/FernandoVillegas27/ai-engineering-os/core/review"
	"github.com/FernandoVillegas27/ai-engineering-os/core/runtimefallback"
	"github.com/FernandoVillegas27/ai-engineering-os/core/staging"
	codexintegration "github.com/FernandoVillegas27/ai-engineering-os/integrations/codex"
	cursorintegration "github.com/FernandoVillegas27/ai-engineering-os/integrations/cursor"
)

type modelRoutingOptions struct {
	Policy   modelpolicy.Policy
	Override string
}

type generatedProposal struct {
	Proposal  proposal.Proposal
	RuntimeID string
	Model     string
	Escalated bool
}

func generateProposalWithRouting(
	ctx context.Context,
	unit execution.WorkUnit,
	runtimeName string,
	options modelRoutingOptions,
) (generatedProposal, error) {
	decision := modelpolicy.Resolve(options.Policy, modelpolicy.RoleProposal, options.Override, false)
	runtime, err := proposalRuntimeFor(runtimeName, decision.Model)
	if err != nil {
		return generatedProposal{}, err
	}

	result, err := (proposal.Generator{Runtime: runtime}).Generate(ctx, unit)
	if err == nil {
		return generatedProposal{
			Proposal: result, RuntimeID: runtime.ID(), Model: decision.Model,
		}, nil
	}

	if !shouldEscalateProposal(runtimeName, options, err) {
		return generatedProposal{}, err
	}

	escalated := modelpolicy.Resolve(options.Policy, modelpolicy.RoleProposal, "", true)
	escalatedRuntime := cursorintegration.NewProposalRuntimeWithModel(escalated.Model)
	retry, retryErr := (proposal.Generator{Runtime: escalatedRuntime}).Generate(ctx, unit)
	if retryErr != nil {
		return generatedProposal{}, fmt.Errorf(
			"proposal auto escalation failed after task-alignment rejection: %w",
			retryErr,
		)
	}
	return generatedProposal{
		Proposal: retry,
		RuntimeID: escalatedRuntime.ID(),
		Model: escalated.Model,
		Escalated: true,
	}, nil
}

func proposalRuntimeFor(runtimeName, cursorModel string) (proposal.Runtime, error) {
	cursorRuntime := cursorintegration.NewProposalRuntimeWithModel(cursorModel)
	switch runtimeName {
	case "auto":
		return runtimefallback.ProposalRuntime{
			Primary: codexintegration.NewProposalRuntime(), Fallback: cursorRuntime,
		}, nil
	case "codex":
		return codexintegration.NewProposalRuntime(), nil
	case "cursor":
		return cursorRuntime, nil
	default:
		return nil, fmt.Errorf("runtime no soportado: %s", runtimeName)
	}
}

func stageRuntimeFor(runtimeName string, options modelRoutingOptions, escalated bool) (staging.Runtime, string, error) {
	decision := modelpolicy.Resolve(options.Policy, modelpolicy.RoleStage, options.Override, escalated)
	cursorRuntime := cursorintegration.NewStageRuntimeWithModel(decision.Model)
	switch runtimeName {
	case "auto":
		return runtimefallback.StageRuntime{
			Primary: codexintegration.NewStageRuntime(), Fallback: cursorRuntime,
		}, decision.Model, nil
	case "codex":
		return codexintegration.NewStageRuntime(), "", nil
	case "cursor":
		return cursorRuntime, decision.Model, nil
	default:
		return nil, "", fmt.Errorf("runtime no soportado: %s", runtimeName)
	}
}

func reviewRuntimeFor(runtimeName string, options modelRoutingOptions, escalated bool) (review.Runtime, string, error) {
	decision := modelpolicy.Resolve(options.Policy, modelpolicy.RoleReview, options.Override, escalated)
	cursorRuntime := cursorintegration.NewReviewRuntimeWithModel(decision.Model)
	switch runtimeName {
	case "auto":
		return runtimefallback.ReviewRuntime{
			Primary: codexintegration.NewReviewRuntime(), Fallback: cursorRuntime,
		}, decision.Model, nil
	case "codex":
		return codexintegration.NewReviewRuntime(), "", nil
	case "cursor":
		return cursorRuntime, decision.Model, nil
	default:
		return nil, "", fmt.Errorf("runtime no soportado: %s", runtimeName)
	}
}

func correctorFor(runtimeName string, options modelRoutingOptions, escalated bool) (quality.Corrector, string, error) {
	decision := modelpolicy.Resolve(options.Policy, modelpolicy.RoleCorrection, options.Override, escalated)
	cursorRuntime := cursorintegration.NewCorrectionRuntimeWithModel(decision.Model)
	switch runtimeName {
	case "auto":
		return runtimefallback.Corrector{
			Primary: codexintegration.NewCorrectionRuntime(), Fallback: cursorRuntime,
		}, decision.Model, nil
	case "codex":
		return codexintegration.NewCorrectionRuntime(), "", nil
	case "cursor":
		return cursorRuntime, decision.Model, nil
	default:
		return nil, "", fmt.Errorf("runtime no soportado: %s", runtimeName)
	}
}

func displayModel(model string) string {
	if model == "" {
		return "auto"
	}
	return model
}


func shouldEscalateProposal(runtimeName string, options modelRoutingOptions, err error) bool {
	if options.Policy != modelpolicy.PolicyAuto || options.Override != "" {
		return false
	}
	if runtimeName != "cursor" && runtimeName != "auto" {
		return false
	}
	return proposal.IsTaskAlignmentError(err)
}
