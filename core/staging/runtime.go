package staging

import (
	"context"

	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/proposal"
)

type StageResult struct {
	Workspace string
	Changes   []WorkspaceChange
	Summary   string
}

type Runtime interface {
	ID() string
	Stage(ctx context.Context, unit execution.WorkUnit, proposal proposal.Proposal) (StageResult, error)
}
