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


func TestExecutionRuntimeRoutingSupportsAutoCodexAndCursor(t *testing.T) {
	options := modelRoutingOptions{Policy: modelpolicy.PolicyAuto}
	cases := []struct {
		name string
		want string
	}{
		{name: "auto", want: "execution-auto"},
		{name: "codex", want: "codex-cli"},
		{name: "cursor", want: "cursor-cli"},
	}
	for _, tc := range cases {
		runtime, _, err := executionRuntimeFor(tc.name, options)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if runtime.ID() != tc.want {
			t.Fatalf("%s runtime id=%q want %q", tc.name, runtime.ID(), tc.want)
		}
	}
}
