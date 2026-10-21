package verification

import (
	"context"
	"time"
)

type Status string

const (
	StatusPassed  Status = "passed"
	StatusFailed  Status = "failed"
	StatusSkipped Status = "skipped"
)

type FailureKind string

const (
	FailureNone           FailureKind = ""
	FailureCode           FailureKind = "code"
	FailureInfrastructure FailureKind = "infrastructure"
)

type Check struct {
	Name        string
	Command     []string
	ExitCode    int
	Stdout      string
	Stderr      string
	Duration    time.Duration
	Passed      bool
	FailureKind FailureKind
	Correctable bool
}

type Report struct {
	Status Status
	Checks []Check
}

type Runtime interface {
	ID() string
	Verify(ctx context.Context, workspace string) (Report, error)
}
