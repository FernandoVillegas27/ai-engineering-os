package runtimefallback

import (
	"context"
	"errors"
	"testing"

	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/proposal"
)

type fakeProposalRuntime struct {
	id       string
	result   proposal.Proposal
	err      error
	calls    int
}

func (f *fakeProposalRuntime) ID() string { return f.id }

func (f *fakeProposalRuntime) Propose(context.Context, execution.WorkUnit) (proposal.Proposal, error) {
	f.calls++
	return f.result, f.err
}

func TestClassifyUsageLimitAsUnavailable(t *testing.T) {
	err := Classify("codex", errors.New("You've hit your usage limit. Upgrade to Pro"))
	if err.Kind != ErrorUnavailable {
		t.Fatalf("kind=%s", err.Kind)
	}
}

func TestProposalFallsBackWhenPrimaryUnavailable(t *testing.T) {
	primary := &fakeProposalRuntime{id: "codex", err: errors.New("usage limit reached")}
	fallback := &fakeProposalRuntime{id: "cursor", result: proposal.Proposal{Summary: "ok"}}
	result, err := (ProposalRuntime{Primary: primary, Fallback: fallback}).Propose(context.Background(), execution.WorkUnit{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Summary != "ok" || primary.calls != 1 || fallback.calls != 1 {
		t.Fatalf("result=%#v primary=%d fallback=%d", result, primary.calls, fallback.calls)
	}
}

func TestProposalDoesNotFallbackForPolicyError(t *testing.T) {
	primary := &fakeProposalRuntime{id: "codex", err: errors.New("declared file is not allowed")}
	fallback := &fakeProposalRuntime{id: "cursor", result: proposal.Proposal{Summary: "should not run"}}
	_, err := (ProposalRuntime{Primary: primary, Fallback: fallback}).Propose(context.Background(), execution.WorkUnit{})
	if err == nil {
		t.Fatal("expected primary error")
	}
	if fallback.calls != 0 {
		t.Fatalf("fallback calls=%d", fallback.calls)
	}
}


type fakeExecutionRuntime struct {
	id     string
	result execution.Analysis
	err    error
	calls  int
}

func (f *fakeExecutionRuntime) ID() string { return f.id }

func (f *fakeExecutionRuntime) Analyze(context.Context, execution.WorkUnit) (execution.Analysis, error) {
	f.calls++
	return f.result, f.err
}

func TestExecutionFallsBackWhenPrimaryUnavailable(t *testing.T) {
	primary := &fakeExecutionRuntime{id: "codex", err: errors.New("usage limit reached")}
	fallback := &fakeExecutionRuntime{id: "cursor", result: execution.Analysis{Summary: "ok"}}
	result, err := (ExecutionRuntime{Primary: primary, Fallback: fallback}).Analyze(context.Background(), execution.WorkUnit{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Summary != "ok" || primary.calls != 1 || fallback.calls != 1 {
		t.Fatalf("result=%#v primary=%d fallback=%d", result, primary.calls, fallback.calls)
	}
}

func TestExecutionDoesNotFallbackForPolicyError(t *testing.T) {
	primary := &fakeExecutionRuntime{id: "codex", err: errors.New("invalid read-only work unit")}
	fallback := &fakeExecutionRuntime{id: "cursor", result: execution.Analysis{Summary: "should not run"}}
	_, err := (ExecutionRuntime{Primary: primary, Fallback: fallback}).Analyze(context.Background(), execution.WorkUnit{})
	if err == nil {
		t.Fatal("expected primary error")
	}
	if fallback.calls != 0 {
		t.Fatalf("fallback calls=%d", fallback.calls)
	}
}
