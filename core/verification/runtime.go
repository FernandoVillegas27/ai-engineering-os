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

type Check struct {
	Name      string
	Command   []string
	ExitCode  int
	Stdout    string
	Stderr    string
	Duration  time.Duration
	Passed    bool
}

type Report struct {
	Status Status
	Checks []Check
}

type Runtime interface {
	ID() string
	Verify(ctx context.Context, workspace string) (Report, error)
}
