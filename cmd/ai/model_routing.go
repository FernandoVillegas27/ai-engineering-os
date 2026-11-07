package main

import (
	"context"
	"fmt"

	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/modelpolicy"
	"github.com/FernandoVillegas27/ai-engineering-os/core/proposal"
	"github.com/FernandoVillegas27/ai-engineering-os/core/quality"
	"github.com/FernandoVillegas27/ai-engineering-os/core/review"
	"github.com/FernandoVillegas27/ai-engineering-os/core/runbudget"
	"github.com/FernandoVillegas27/ai-engineering-os/core/runtimefallback"
	"github.com/FernandoVillegas27/ai-engineering-os/core/staging"
	codexintegration "github.com/FernandoVillegas27/ai-engineering-os/integrations/codex"
	cursorintegration "github.com/FernandoVillegas27/ai-engineering-os/integrations/cursor"
)

type modelRoutingOptions struct {
	Policy   modelpolicy.Policy
	Override string
	Budget   *runbudget.Budget
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
	runtime, err := proposalRuntimeFor(runtimeName, decision.Model, options.Budget)
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
	escalatedRuntime := proposal.Runtime(cursorintegration.NewProposalRuntimeWithModel(escalated.Model))
	if options.Budget != nil {
		escalatedRuntime = runbudget.ProposalRuntime{Budget: options.Budget, Inner: escalatedRuntime}
	}
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

func proposalRuntimeFor(runtimeName, cursorModel string, budget ...*runbudget.Budget) (proposal.Runtime, error) {
	var shared *runbudget.Budget
	if len(budget) > 0 { shared = budget[0] }
	wrap := func(inner proposal.Runtime) proposal.Runtime {
		if shared == nil { return inner }
		return runbudget.ProposalRuntime{Budget: shared, Inner: inner}
	}
	cursorRuntime := wrap(cursorintegration.NewProposalRuntimeWithModel(cursorModel))
	switch runtimeName {
	case "auto":
		return runtimefallback.ProposalRuntime{
			Primary: wrap(codexintegration.NewProposalRuntime()), Fallback: cursorRuntime,
		}, nil
	case "codex":
		return wrap(codexintegration.NewProposalRuntime()), nil
	case "cursor":
		return cursorRuntime, nil
	default:
		return nil, fmt.Errorf("runtime no soportado: %s", runtimeName)
	}
}

func executionRuntimeFor(runtimeName string, options modelRoutingOptions) (execution.Runtime, string, error) {
	decision := modelpolicy.Resolve(options.Policy, modelpolicy.RoleExecution, options.Override, false)
	wrap := func(inner execution.Runtime) execution.Runtime {
		if options.Budget == nil { return inner }
		return runbudget.ExecutionRuntime{Budget: options.Budget, Inner: inner}
	}
	cursorRuntime := wrap(cursorintegration.NewExecutionRuntimeWithModel(decision.Model))
	switch runtimeName {
	case "auto":
		codexRuntime := codexintegration.NewExecutionRuntime()
		codexRuntime.Model = decision.Model
		return runtimefallback.ExecutionRuntime{
			Primary: wrap(codexRuntime),
			Fallback: cursorRuntime,
		}, decision.Model, nil
	case "codex":
		codexRuntime := codexintegration.NewExecutionRuntime()
		codexRuntime.Model = decision.Model
		return wrap(codexRuntime), decision.Model, nil
	case "cursor":
		return cursorRuntime, decision.Model, nil
	default:
		return nil, "", fmt.Errorf("runtime no soportado: %s", runtimeName)
	}
}

func stageRuntimeFor(runtimeName string, options modelRoutingOptions, escalated bool) (staging.Runtime, string, error) {
	decision := modelpolicy.Resolve(options.Policy, modelpolicy.RoleStage, options.Override, escalated)
	wrap := func(inner staging.Runtime) staging.Runtime {
		if options.Budget == nil { return inner }
		return runbudget.StageRuntime{Budget: options.Budget, Inner: inner}
	}
	cursorRuntime := wrap(cursorintegration.NewStageRuntimeWithModel(decision.Model))
	switch runtimeName {
	case "auto":
		return runtimefallback.StageRuntime{
			Primary: wrap(codexintegration.NewStageRuntime()), Fallback: cursorRuntime,
		}, decision.Model, nil
	case "codex":
		return wrap(codexintegration.NewStageRuntime()), "", nil
	case "cursor":
		return cursorRuntime, decision.Model, nil
	default:
		return nil, "", fmt.Errorf("runtime no soportado: %s", runtimeName)
	}
}

func reviewRuntimeFor(runtimeName string, options modelRoutingOptions, escalated bool) (review.Runtime, string, error) {
	decision := modelpolicy.Resolve(options.Policy, modelpolicy.RoleReview, options.Override, escalated)
	wrap := func(inner review.Runtime) review.Runtime {
		if options.Budget == nil { return inner }
		return runbudget.ReviewRuntime{Budget: options.Budget, Inner: inner}
	}
	cursorRuntime := wrap(cursorintegration.NewReviewRuntimeWithModel(decision.Model))
	switch runtimeName {
	case "auto":
		return runtimefallback.ReviewRuntime{
			Primary: wrap(codexintegration.NewReviewRuntime()), Fallback: cursorRuntime,
		}, decision.Model, nil
	case "codex":
		return wrap(codexintegration.NewReviewRuntime()), "", nil
	case "cursor":
		return cursorRuntime, decision.Model, nil
	default:
		return nil, "", fmt.Errorf("runtime no soportado: %s", runtimeName)
	}
}

func correctorFor(runtimeName string, options modelRoutingOptions, escalated bool) (quality.Corrector, string, error) {
	decision := modelpolicy.Resolve(options.Policy, modelpolicy.RoleCorrection, options.Override, escalated)
	wrap := func(inner quality.Corrector) quality.Corrector {
		if options.Budget == nil { return inner }
		return runbudget.Corrector{Budget: options.Budget, Inner: inner}
	}
	cursorRuntime := wrap(cursorintegration.NewCorrectionRuntimeWithModel(decision.Model))
	switch runtimeName {
	case "auto":
		return runtimefallback.Corrector{
			Primary: wrap(codexintegration.NewCorrectionRuntime()), Fallback: cursorRuntime,
		}, decision.Model, nil
	case "codex":
		return wrap(codexintegration.NewCorrectionRuntime()), "", nil
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
