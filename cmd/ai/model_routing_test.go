package main

import (
	"fmt"
	"testing"

	"github.com/FernandoVillegas27/ai-engineering-os/core/modelpolicy"
	"github.com/FernandoVillegas27/ai-engineering-os/core/proposal"
)

func TestShouldEscalateProposalOnAutoTaskDrift(t *testing.T) {
	err := fmt.Errorf("wrapped: %w", proposal.TaskAlignmentError{Detail: "missing 5"})
	if !shouldEscalateProposal("cursor", modelRoutingOptions{Policy: modelpolicy.PolicyAuto}, err) {
		t.Fatal("expected auto Cursor proposal to escalate after task drift")
	}
}

func TestShouldEscalateProposalForAutoRuntimeFallback(t *testing.T) {
	err := proposal.TaskAlignmentError{Detail: "missing timeout"}
	if !shouldEscalateProposal("auto", modelRoutingOptions{Policy: modelpolicy.PolicyAuto}, err) {
		t.Fatal("expected auto runtime proposal to escalate after task drift")
	}
}

func TestShouldNotEscalateEconomyPolicy(t *testing.T) {
	err := proposal.TaskAlignmentError{Detail: "missing 5"}
	if shouldEscalateProposal("cursor", modelRoutingOptions{Policy: modelpolicy.PolicyEconomy}, err) {
		t.Fatal("economy policy must not auto-escalate")
	}
}

func TestShouldNotEscalateExplicitModel(t *testing.T) {
	err := proposal.TaskAlignmentError{Detail: "missing 5"}
	if shouldEscalateProposal("cursor", modelRoutingOptions{
		Policy: modelpolicy.PolicyAuto,
		Override: "custom-model",
	}, err) {
		t.Fatal("explicit model must not be automatically replaced")
	}
}

func TestShouldNotEscalateCodexOnlyRuntime(t *testing.T) {
	err := proposal.TaskAlignmentError{Detail: "missing 5"}
	if shouldEscalateProposal("codex", modelRoutingOptions{Policy: modelpolicy.PolicyAuto}, err) {
		t.Fatal("codex-only runtime must not switch providers through model escalation")
	}
}

func TestShouldNotEscalateUnrelatedError(t *testing.T) {
	if shouldEscalateProposal("cursor", modelRoutingOptions{Policy: modelpolicy.PolicyAuto}, fmt.Errorf("invalid path")) {
		t.Fatal("non-alignment errors must not trigger model escalation")
	}
}
