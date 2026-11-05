package coordinator

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/orchestration"
	"github.com/FernandoVillegas27/ai-engineering-os/core/scheduler"
)

type fakeExecutionExecutor struct {
	result execution.Result
	err    error
	calls  []execution.WorkUnit
}

func (f *fakeExecutionExecutor) Execute(_ context.Context, unit execution.WorkUnit) (execution.Result, error) {
	f.calls = append(f.calls, unit)
	return f.result, f.err
}

func TestExecutionWorkerRunsMatchingWorkUnit(t *testing.T) {
	executor := &fakeExecutionExecutor{result: execution.Result{Summary: "done"}}
	unit := execution.WorkUnit{
		ID:      "work-01-backend",
		AgentID: "backend-engineer",
		Intent:  orchestration.IntentResearch,
		Mode:    execution.ModeReadOnly,
	}
	worker := ExecutionWorker{
		Executor: executor,
		Units: map[string]execution.WorkUnit{
			unit.ID: unit,
		},
	}

	result, err := worker.Execute(context.Background(), scheduler.Task{
		ID:      unit.ID,
		AgentID: unit.AgentID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Summary != "done" || len(executor.calls) != 1 || executor.calls[0].ID != unit.ID {
		t.Fatalf("result=%#v calls=%#v", result, executor.calls)
	}
}

func TestExecutionWorkerRejectsMissingOrMismatchedUnit(t *testing.T) {
	executor := &fakeExecutionExecutor{}
	worker := ExecutionWorker{Executor: executor, Units: map[string]execution.WorkUnit{}}
	if _, err := worker.Execute(context.Background(), scheduler.Task{ID: "missing", AgentID: "architect"}); err == nil {
		t.Fatal("expected missing work unit rejection")
	}

	unit := execution.WorkUnit{ID: "work-1", AgentID: "architect", Mode: execution.ModeReadOnly}
	worker.Units[unit.ID] = unit
	if _, err := worker.Execute(context.Background(), scheduler.Task{ID: unit.ID, AgentID: "backend-engineer"}); err == nil {
		t.Fatal("expected agent mismatch rejection")
	}
}

func TestExecutionWorkerPropagatesExecutorError(t *testing.T) {
	executor := &fakeExecutionExecutor{err: errors.New("runtime failed")}
	unit := execution.WorkUnit{ID: "work-1", AgentID: "researcher", Mode: execution.ModeReadOnly}
	worker := ExecutionWorker{
		Executor: executor,
		Units:    map[string]execution.WorkUnit{unit.ID: unit},
	}
	if _, err := worker.Execute(context.Background(), scheduler.Task{ID: unit.ID, AgentID: unit.AgentID}); err == nil {
		t.Fatal("expected executor error")
	}
}


type fakeWorkContextProvider struct {
	value string
	err   error
	calls int
}

func (f *fakeWorkContextProvider) Context(context.Context, scheduler.Task) (string, error) {
	f.calls++
	return f.value, f.err
}

func TestExecutionWorkerInjectsCollaborationContextIntoUnitCopy(t *testing.T) {
	executor := &fakeExecutionExecutor{}
	provider := &fakeWorkContextProvider{value: "handoff context"}
	unit := execution.WorkUnit{
		ID:                   "work-1",
		AgentID:              "backend-engineer",
		Mode:                 execution.ModeReadOnly,
		CollaborationContext: "durable memory",
	}
	worker := ExecutionWorker{
		Executor:        executor,
		Units:           map[string]execution.WorkUnit{unit.ID: unit},
		ContextProvider: provider,
	}
	if _, err := worker.Execute(context.Background(), scheduler.Task{ID: unit.ID, AgentID: unit.AgentID}); err != nil {
		t.Fatal(err)
	}
	if provider.calls != 1 || len(executor.calls) != 1 {
		t.Fatalf("provider calls=%d executor calls=%d", provider.calls, len(executor.calls))
	}
	if executor.calls[0].CollaborationContext != "durable memory\n\nhandoff context" {
		t.Fatalf("context = %q", executor.calls[0].CollaborationContext)
	}
	if unit.CollaborationContext != "durable memory" {
		t.Fatalf("original unit was mutated: %#v", unit)
	}
}


func TestMergeContextBoundsCombinedEvidence(t *testing.T) {
	value := MergeContext(strings.Repeat("a", 100), strings.Repeat("b", 100), 80)
	if len(value) != 80 || !strings.HasSuffix(value, "...") {
		t.Fatalf("value len=%d value=%q", len(value), value)
	}
}
