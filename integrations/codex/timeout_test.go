package codex

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/proposal"
	"github.com/FernandoVillegas27/ai-engineering-os/core/quality"
)

func TestResolveRuntimeTimeout(t *testing.T) {
	tests := []struct {
		name       string
		configured time.Duration
		want       time.Duration
		wantError  bool
	}{
		{name: "zero uses default", configured: 0, want: defaultRuntimeTimeout},
		{name: "minimum accepted", configured: minimumRuntimeTimeout, want: minimumRuntimeTimeout},
		{name: "above minimum accepted", configured: 30 * time.Second, want: 30 * time.Second},
		{name: "below minimum rejected", configured: minimumRuntimeTimeout - time.Nanosecond, wantError: true},
		{name: "negative rejected", configured: -time.Second, wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveRuntimeTimeout(tt.configured)
			if tt.wantError {
				if err == nil {
					t.Fatalf("resolveRuntimeTimeout(%s) expected error", tt.configured)
				}
				if !strings.Contains(err.Error(), "at least 5s") {
					t.Fatalf("resolveRuntimeTimeout(%s) error = %q", tt.configured, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveRuntimeTimeout(%s) error = %v", tt.configured, err)
			}
			if got != tt.want {
				t.Fatalf("resolveRuntimeTimeout(%s) = %s, want %s", tt.configured, got, tt.want)
			}
		})
	}
}

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

func TestRuntimeEntryPointsRejectTimeoutBelowMinimum(t *testing.T) {
	root := t.TempDir()
	unit := executionUnitFixture(root)
	invalidTimeout := minimumRuntimeTimeout - time.Nanosecond

	proposalRunner := &executionFakeRunner{}
	stageRunner := &executionFakeRunner{}
	correctionRunner := &executionFakeRunner{}

	tests := []struct {
		name   string
		runner *executionFakeRunner
		run    func() error
	}{
		{
			name:   "proposal",
			runner: proposalRunner,
			run: func() error {
				_, err := (ProposalRuntime{
					Runner:  proposalRunner,
					Timeout: invalidTimeout,
				}).Propose(context.Background(), unit)
				return err
			},
		},
		{
			name:   "stage",
			runner: stageRunner,
			run: func() error {
				_, err := (StageRuntime{
					Runner:  stageRunner,
					Timeout: invalidTimeout,
				}).Stage(context.Background(), unit, proposal.Proposal{})
				return err
			},
		},
		{
			name:   "correction",
			runner: correctionRunner,
			run: func() error {
				return (CorrectionRuntime{
					Runner:  correctionRunner,
					Timeout: invalidTimeout,
				}).Correct(context.Background(), quality.CorrectionRequest{
					Workspace:    root,
					AllowedFiles: []string{"main.go"},
				})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run()
			if err == nil || !strings.Contains(err.Error(), "at least 5s") {
				t.Fatalf("expected minimum timeout error, got %v", err)
			}
			if tt.runner.invocation.Command != "" {
				t.Fatal("runner must not be invoked for an invalid timeout")
			}
		})
	}
}
