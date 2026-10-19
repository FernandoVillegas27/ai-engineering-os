package execution

import (
	"context"
	"errors"
	"testing"
)

type fakeRuntime struct {
	id       string
	analysis Analysis
	err      error
	called   bool
}

func (f *fakeRuntime) ID() string { return f.id }

func (f *fakeRuntime) Analyze(_ context.Context, _ WorkUnit) (Analysis, error) {
	f.called = true
	return f.analysis, f.err
}

func readOnlyUnit() WorkUnit {
	return WorkUnit{
		ID:      "work-01-reviewer",
		AgentID: "reviewer",
		Mode:    ModeReadOnly,
		Permissions: PermissionEnvelope{
			SourceCode: "read",
			Tests:      "read",
			GitCommit:  "deny",
			GitPush:    "deny",
			Secrets:    "deny",
			Production: "deny",
		},
	}
}

func TestRuntimeExecutorExecutesReadOnlyRuntime(t *testing.T) {
	runtime := &fakeRuntime{
		id: "fake-runtime",
		analysis: Analysis{
			Summary: "review complete",
			Findings: []Finding{{
				Severity: "medium",
				Title:    "Missing authorization check",
				Detail:   "Handler relies on caller filtering.",
			}},
			Evidence: []Evidence{{
				Path:   "api/handler.go",
				Detail: "No authorization guard before update.",
			}},
		},
	}

	result, err := (RuntimeExecutor{Runtime: runtime}).Execute(context.Background(), readOnlyUnit())
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !runtime.called {
		t.Fatal("expected runtime to be called")
	}
	if !result.Executed || result.Status != StatusCompleted {
		t.Fatalf("result = %#v", result)
	}
	if result.Runtime != "fake-runtime" {
		t.Fatalf("runtime = %q", result.Runtime)
	}
	if len(result.Changes) != 0 {
		t.Fatalf("changes = %#v", result.Changes)
	}
	if len(result.Findings) != 1 || len(result.Evidence) != 1 {
		t.Fatalf("analysis not propagated: %#v", result)
	}
}

func TestRuntimeExecutorRejectsWriteEnvelope(t *testing.T) {
	unit := readOnlyUnit()
	unit.Permissions.SourceCode = "write"
	runtime := &fakeRuntime{id: "fake-runtime"}

	_, err := (RuntimeExecutor{Runtime: runtime}).Execute(context.Background(), unit)
	if err == nil {
		t.Fatal("expected permission error")
	}
	if runtime.called {
		t.Fatal("runtime must not be called when permissions are invalid")
	}
}

func TestRuntimeExecutorPropagatesRuntimeFailure(t *testing.T) {
	runtime := &fakeRuntime{id: "fake-runtime", err: errors.New("failed")}
	_, err := (RuntimeExecutor{Runtime: runtime}).Execute(context.Background(), readOnlyUnit())
	if err == nil {
		t.Fatal("expected runtime error")
	}
}
