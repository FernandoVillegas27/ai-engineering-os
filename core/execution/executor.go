package execution

import "context"

type Status string

const (
	StatusDryRun    Status = "dry-run"
	StatusCompleted Status = "completed"
)

type Result struct {
	WorkUnitID string
	AgentID    string
	Status     Status
	Mode       Mode
	Summary    string
	Findings   []Finding
	Evidence   []Evidence
	Changes    []string
	Executed   bool
	Runtime    string
}

type Executor interface {
	Execute(ctx context.Context, unit WorkUnit) (Result, error)
}

type DryRunExecutor struct{}

func (DryRunExecutor) Execute(_ context.Context, unit WorkUnit) (Result, error) {
	if err := validateReadOnlyUnit(unit); err != nil {
		return Result{}, err
	}

	return Result{
		WorkUnitID: unit.ID,
		AgentID:    unit.AgentID,
		Status:     StatusDryRun,
		Mode:       unit.Mode,
		Summary:    "Work unit validated. No runtime agent was invoked and no project changes were made.",
		Findings:   []Finding{},
		Evidence:   []Evidence{},
		Changes:    []string{},
		Executed:   false,
	}, nil
}
