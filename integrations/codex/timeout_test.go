package codex

import (
	"testing"
	"time"
)

func TestDefaultRuntimeTimeoutBudget(t *testing.T) {
	if defaultRuntimeTimeout != 5*time.Minute {
		t.Fatalf("defaultRuntimeTimeout = %s, want 5m", defaultRuntimeTimeout)
	}

	if got := NewProposalRuntime().Timeout; got != defaultRuntimeTimeout {
		t.Fatalf("proposal timeout = %s, want %s", got, defaultRuntimeTimeout)
	}
	if got := NewExecutionRuntime().Timeout; got != defaultRuntimeTimeout {
		t.Fatalf("execution timeout = %s, want %s", got, defaultRuntimeTimeout)
	}
	if got := NewStageRuntime().Timeout; got != defaultRuntimeTimeout {
		t.Fatalf("stage timeout = %s, want %s", got, defaultRuntimeTimeout)
	}
	if got := NewCorrectionRuntime().Timeout; got != defaultRuntimeTimeout {
		t.Fatalf("correction timeout = %s, want %s", got, defaultRuntimeTimeout)
	}
}
