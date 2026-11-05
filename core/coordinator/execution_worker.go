package coordinator

import (
	"context"
	"errors"
	"fmt"

	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/scheduler"
)

type WorkContextProvider interface {
	Context(ctx context.Context, task scheduler.Task) (string, error)
}

type ExecutionWorker struct {
	Executor        execution.Executor
	Units           map[string]execution.WorkUnit
	ContextProvider WorkContextProvider
}

func (w ExecutionWorker) Execute(ctx context.Context, task scheduler.Task) (WorkResult, error) {
	if w.Executor == nil {
		return WorkResult{}, errors.New("execution worker requires executor")
	}
	unit, ok := w.Units[task.ID]
	if !ok {
		return WorkResult{}, fmt.Errorf("work unit not found for scheduler task %s", task.ID)
	}
	if unit.AgentID != task.AgentID {
		return WorkResult{}, fmt.Errorf("agent mismatch for task %s: scheduler=%s work-unit=%s", task.ID, task.AgentID, unit.AgentID)
	}
	if w.ContextProvider != nil {
		contextValue, err := w.ContextProvider.Context(ctx, task)
		if err != nil {
			return WorkResult{}, fmt.Errorf("load collaboration context for %s: %w", task.ID, err)
		}
		unit.CollaborationContext = contextValue
	}
	result, err := w.Executor.Execute(ctx, unit)
	if err != nil {
		return WorkResult{}, err
	}
	return WorkResult{
		Summary: result.Summary,
	}, nil
}
