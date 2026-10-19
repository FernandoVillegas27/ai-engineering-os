package execution

import (
	"context"
	"fmt"
)

type Status string

const (
	StatusDryRun Status = "dry-run"
)

type Result struct {
	WorkUnitID string
	AgentID    string
	Status     Status
	Mode       Mode
	Summary    string
	Changes    []string
	Executed   bool
}

type Executor interface {
	Execute(ctx context.Context, unit WorkUnit) (Result, error)
}

type DryRunExecutor struct{}

func (DryRunExecutor) Execute(_ context.Context, unit WorkUnit) (Result, error) {
	if unit.ID == "" || unit.AgentID == "" {
		return Result{}, fmt.Errorf("invalid work unit")
	}
	if unit.Mode != ModeReadOnly {
		return Result{}, fmt.Errorf("dry-run executor only accepts read-only work units")
	}
	if unit.Permissions.SourceCode == "write" ||
		unit.Permissions.Tests == "write" ||
		unit.Permissions.GitCommit != "deny" ||
		unit.Permissions.GitPush != "deny" ||
		unit.Permissions.Secrets != "deny" ||
		unit.Permissions.Production != "deny" {
		return Result{}, fmt.Errorf("work unit exceeds dry-run permission envelope")
	}

	return Result{
		WorkUnitID: unit.ID,
		AgentID:    unit.AgentID,
		Status:     StatusDryRun,
		Mode:       unit.Mode,
		Summary:    "Work unit validated. No runtime agent was invoked and no project changes were made.",
		Changes:    []string{},
		Executed:   false,
	}, nil
}
