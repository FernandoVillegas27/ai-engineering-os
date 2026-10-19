package execution

import (
	"context"
	"fmt"
)

type Finding struct {
	Severity string
	Title    string
	Detail   string
}

type Evidence struct {
	Path   string
	Detail string
}

type Analysis struct {
	Summary  string
	Findings []Finding
	Evidence []Evidence
}

type Runtime interface {
	ID() string
	Analyze(ctx context.Context, unit WorkUnit) (Analysis, error)
}

type RuntimeExecutor struct {
	Runtime Runtime
}

func (e RuntimeExecutor) Execute(ctx context.Context, unit WorkUnit) (Result, error) {
	if e.Runtime == nil {
		return Result{}, fmt.Errorf("runtime is required")
	}
	if err := validateReadOnlyUnit(unit); err != nil {
		return Result{}, err
	}

	analysis, err := e.Runtime.Analyze(ctx, unit)
	if err != nil {
		return Result{}, fmt.Errorf("runtime %s: %w", e.Runtime.ID(), err)
	}

	return Result{
		WorkUnitID: unit.ID,
		AgentID:    unit.AgentID,
		Status:     StatusCompleted,
		Mode:       unit.Mode,
		Summary:    analysis.Summary,
		Findings:   analysis.Findings,
		Evidence:   analysis.Evidence,
		Changes:    []string{},
		Executed:   true,
		Runtime:    e.Runtime.ID(),
	}, nil
}

func validateReadOnlyUnit(unit WorkUnit) error {
	if unit.ID == "" || unit.AgentID == "" {
		return fmt.Errorf("invalid work unit")
	}
	if unit.Mode != ModeReadOnly {
		return fmt.Errorf("runtime executor only accepts read-only work units")
	}
	if unit.Permissions.SourceCode == "write" ||
		unit.Permissions.Tests == "write" ||
		unit.Permissions.GitCommit != "deny" ||
		unit.Permissions.GitPush != "deny" ||
		unit.Permissions.Secrets != "deny" ||
		unit.Permissions.Production != "deny" {
		return fmt.Errorf("work unit exceeds read-only runtime permission envelope")
	}
	return nil
}
