package runbudget

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/proposal"
)

type fakeExecution struct{ calls int }
func (f *fakeExecution) ID() string { return "fake-execution" }
func (f *fakeExecution) Analyze(context.Context, execution.WorkUnit) (execution.Analysis, error) {
	f.calls++
	return execution.Analysis{Summary: "ok"}, nil
}

type fakeProposal struct{ calls int }
func (f *fakeProposal) ID() string { return "fake-proposal" }
func (f *fakeProposal) Propose(context.Context, execution.WorkUnit) (proposal.Proposal, error) {
	f.calls++
	return proposal.Proposal{Summary: "ok"}, nil
}

func TestInvocationBudgetBlocksBeforeProviderCall(t *testing.T) {
	budget, err := New(Config{MaxInvocations: 1, MaxDuration: time.Minute})
	if err != nil { t.Fatal(err) }
	inner := &fakeExecution{}
	runtime := ExecutionRuntime{Budget: budget, Inner: inner}
	unit := execution.WorkUnit{ID: "w", AgentID: "a"}

	if _, err := runtime.Analyze(context.Background(), unit); err != nil { t.Fatal(err) }
	if _, err := runtime.Analyze(context.Background(), unit); err == nil {
		t.Fatal("expected invocation limit")
	} else {
		var budgetErr *Error
		if !errors.As(err, &budgetErr) || budgetErr.Kind != KindInvocationLimit {
			t.Fatalf("err=%#v", err)
		}
	}
	if inner.calls != 1 {
		t.Fatalf("provider calls=%d want=1", inner.calls)
	}
	snapshot := budget.Snapshot()
	if snapshot.UsedInvocations != 1 || snapshot.Remaining != 0 {
		t.Fatalf("snapshot=%#v", snapshot)
	}
}

func TestDifferentRuntimeTypesShareSameCounter(t *testing.T) {
	budget, _ := New(Config{MaxInvocations: 2, MaxDuration: time.Minute})
	executionInner := &fakeExecution{}
	proposalInner := &fakeProposal{}
	executionRuntime := ExecutionRuntime{Budget: budget, Inner: executionInner}
	proposalRuntime := ProposalRuntime{Budget: budget, Inner: proposalInner}

	if _, err := executionRuntime.Analyze(context.Background(), execution.WorkUnit{}); err != nil { t.Fatal(err) }
	if _, err := proposalRuntime.Propose(context.Background(), execution.WorkUnit{}); err != nil { t.Fatal(err) }
	if _, err := proposalRuntime.Propose(context.Background(), execution.WorkUnit{}); err == nil {
		t.Fatal("expected shared invocation limit")
	}
	if executionInner.calls != 1 || proposalInner.calls != 1 {
		t.Fatalf("execution=%d proposal=%d", executionInner.calls, proposalInner.calls)
	}
}

func TestBudgetContextExpiresAtGlobalDeadline(t *testing.T) {
	budget, err := New(Config{MaxInvocations: 5, MaxDuration: 30*time.Millisecond})
	if err != nil { t.Fatal(err) }
	ctx, cancel := budget.Context(context.Background())
	defer cancel()
	select {
	case <-ctx.Done():
		if !errors.Is(ctx.Err(), context.DeadlineExceeded) {
			t.Fatalf("err=%v", ctx.Err())
		}
	case <-time.After(time.Second):
		t.Fatal("budget context did not expire")
	}
	if err := budget.Acquire("after-deadline"); err == nil {
		t.Fatal("expected runtime budget exhaustion")
	} else {
		var budgetErr *Error
		if !errors.As(err, &budgetErr) || budgetErr.Kind != KindRuntimeLimit {
			t.Fatalf("err=%#v", err)
		}
	}
}
