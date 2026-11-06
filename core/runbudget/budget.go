package runbudget

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/proposal"
	"github.com/FernandoVillegas27/ai-engineering-os/core/quality"
	"github.com/FernandoVillegas27/ai-engineering-os/core/review"
	"github.com/FernandoVillegas27/ai-engineering-os/core/staging"
)

type Kind string

const (
	KindInvocationLimit Kind = "invocation-limit"
	KindRuntimeLimit    Kind = "runtime-limit"
)

var ErrBudget = errors.New("run budget exhausted")

type Error struct {
	Kind      Kind
	Operation string
	Used      int
	Limit     int
	Duration  time.Duration
}

func (e *Error) Error() string {
	switch e.Kind {
	case KindInvocationLimit:
		return fmt.Sprintf(
			"run budget exhausted (%s): operation=%s used=%d limit=%d",
			e.Kind, e.Operation, e.Used, e.Limit,
		)
	case KindRuntimeLimit:
		return fmt.Sprintf(
			"run budget exhausted (%s): operation=%s duration=%s",
			e.Kind, e.Operation, e.Duration,
		)
	default:
		return "run budget exhausted"
	}
}

func (e *Error) Is(target error) bool { return target == ErrBudget }

type Config struct {
	MaxInvocations int
	MaxDuration    time.Duration
}

type Snapshot struct {
	MaxInvocations int           `json:"max_invocations"`
	UsedInvocations int          `json:"used_invocations"`
	Remaining       int          `json:"remaining_invocations"`
	MaxDuration     time.Duration `json:"max_duration"`
	Elapsed         time.Duration `json:"elapsed"`
}

type Budget struct {
	mu             sync.Mutex
	maxInvocations int
	used           int
	maxDuration    time.Duration
	started        time.Time
}

func New(config Config) (*Budget, error) {
	if config.MaxInvocations < 1 {
		return nil, errors.New("run budget max invocations must be >= 1")
	}
	if config.MaxDuration <= 0 {
		return nil, errors.New("run budget max duration must be > 0")
	}
	return &Budget{
		maxInvocations: config.MaxInvocations,
		maxDuration: config.MaxDuration,
		started: time.Now(),
	}, nil
}

func (b *Budget) Acquire(operation string) error {
	if b == nil {
		return errors.New("run budget is required")
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	elapsed := time.Since(b.started)
	if elapsed >= b.maxDuration {
		return &Error{
			Kind: KindRuntimeLimit, Operation: operation,
			Used: b.used, Limit: b.maxInvocations, Duration: elapsed,
		}
	}
	if b.used >= b.maxInvocations {
		return &Error{
			Kind: KindInvocationLimit, Operation: operation,
			Used: b.used, Limit: b.maxInvocations, Duration: elapsed,
		}
	}
	b.used++
	return nil
}

func (b *Budget) Context(parent context.Context) (context.Context, context.CancelFunc) {
	if b == nil {
		return context.WithCancel(parent)
	}
	b.mu.Lock()
	deadline := b.started.Add(b.maxDuration)
	b.mu.Unlock()
	return context.WithDeadline(parent, deadline)
}

func (b *Budget) Snapshot() Snapshot {
	if b == nil {
		return Snapshot{}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	remaining := b.maxInvocations - b.used
	if remaining < 0 {
		remaining = 0
	}
	return Snapshot{
		MaxInvocations: b.maxInvocations,
		UsedInvocations: b.used,
		Remaining: remaining,
		MaxDuration: b.maxDuration,
		Elapsed: time.Since(b.started),
	}
}

type ExecutionRuntime struct {
	Budget *Budget
	Inner  execution.Runtime
}

func (r ExecutionRuntime) ID() string {
	if r.Inner == nil { return "budgeted-execution" }
	return r.Inner.ID()
}
func (r ExecutionRuntime) Analyze(ctx context.Context, unit execution.WorkUnit) (execution.Analysis, error) {
	if r.Inner == nil { return execution.Analysis{}, errors.New("execution runtime is required") }
	if err := r.Budget.Acquire("execution:"+r.Inner.ID()); err != nil { return execution.Analysis{}, err }
	return r.Inner.Analyze(ctx, unit)
}

type ProposalRuntime struct {
	Budget *Budget
	Inner  proposal.Runtime
}
func (r ProposalRuntime) ID() string {
	if r.Inner == nil { return "budgeted-proposal" }
	return r.Inner.ID()
}
func (r ProposalRuntime) Propose(ctx context.Context, unit execution.WorkUnit) (proposal.Proposal, error) {
	if r.Inner == nil { return proposal.Proposal{}, errors.New("proposal runtime is required") }
	if err := r.Budget.Acquire("proposal:"+r.Inner.ID()); err != nil { return proposal.Proposal{}, err }
	return r.Inner.Propose(ctx, unit)
}

type StageRuntime struct {
	Budget *Budget
	Inner  staging.Runtime
}
func (r StageRuntime) ID() string {
	if r.Inner == nil { return "budgeted-stage" }
	return r.Inner.ID()
}
func (r StageRuntime) Stage(ctx context.Context, unit execution.WorkUnit, p proposal.Proposal) (staging.StageResult, error) {
	if r.Inner == nil { return staging.StageResult{}, errors.New("stage runtime is required") }
	if err := r.Budget.Acquire("stage:"+r.Inner.ID()); err != nil { return staging.StageResult{}, err }
	return r.Inner.Stage(ctx, unit, p)
}

type ReviewRuntime struct {
	Budget *Budget
	Inner  review.Runtime
}
func (r ReviewRuntime) ID() string {
	if r.Inner == nil { return "budgeted-review" }
	return r.Inner.ID()
}
func (r ReviewRuntime) Review(ctx context.Context, workspace, task string, changedFiles []string) (review.Report, error) {
	if r.Inner == nil { return review.Report{}, errors.New("review runtime is required") }
	if err := r.Budget.Acquire("review:"+r.Inner.ID()); err != nil { return review.Report{}, err }
	return r.Inner.Review(ctx, workspace, task, changedFiles)
}

type Corrector struct {
	Budget *Budget
	Inner  quality.Corrector
}
func (r Corrector) ID() string {
	if r.Inner == nil { return "budgeted-correction" }
	return r.Inner.ID()
}
func (r Corrector) Correct(ctx context.Context, request quality.CorrectionRequest) error {
	if r.Inner == nil { return errors.New("corrector is required") }
	if err := r.Budget.Acquire("correction:"+r.Inner.ID()); err != nil { return err }
	return r.Inner.Correct(ctx, request)
}
