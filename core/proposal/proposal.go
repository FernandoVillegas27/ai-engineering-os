package proposal

import (
	"context"
	"fmt"
	"strings"

	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
)

type FileChange struct {
	Path      string
	Action    string
	Rationale string
}

type Risk struct {
	Severity string
	Detail   string
}

type Proposal struct {
	Summary      string
	Files        []FileChange
	Patch        string
	Risks        []Risk
	Verification []string
}

type Runtime interface {
	ID() string
	Propose(ctx context.Context, unit execution.WorkUnit) (Proposal, error)
}

type Generator struct {
	Runtime Runtime
}

func (g Generator) Generate(ctx context.Context, unit execution.WorkUnit) (Proposal, error) {
	if g.Runtime == nil {
		return Proposal{}, fmt.Errorf("proposal runtime is required")
	}
	if err := execution.ValidateReadOnlyUnit(unit); err != nil {
		return Proposal{}, err
	}

	proposal, err := g.Runtime.Propose(ctx, unit)
	if err != nil {
		return Proposal{}, fmt.Errorf("proposal runtime %s: %w", g.Runtime.ID(), err)
	}
	if strings.TrimSpace(proposal.Summary) == "" {
		return Proposal{}, fmt.Errorf("proposal runtime %s returned an empty summary", g.Runtime.ID())
	}
	return proposal, nil
}
