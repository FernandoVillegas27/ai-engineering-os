package mutation

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/proposal"
	"github.com/FernandoVillegas27/ai-engineering-os/core/staging"
)

type ProposalRuntimeResolver func(unit execution.WorkUnit) (proposal.Runtime, error)

type FreezeConfig struct {
	MaxConcurrency int
	Policy         execution.SafeViewPolicy
}

type FrozenProposal struct {
	TaskID   string                    `json:"task_id"`
	AgentID  string                    `json:"agent_id"`
	Proposal proposal.Proposal         `json:"proposal"`
	Declared []staging.WorkspaceChange `json:"declared"`
}

type proposalResult struct {
	frozen FrozenProposal
	err    error
}

func FreezeProposals(
	ctx context.Context,
	units []execution.WorkUnit,
	resolve ProposalRuntimeResolver,
	config FreezeConfig,
) ([]FrozenProposal, error) {
	if len(units) == 0 {
		return nil, errors.New("proposal freeze requires at least one work unit")
	}
	if resolve == nil {
		return nil, errors.New("proposal freeze requires a runtime resolver")
	}

	seenUnits := map[string]bool{}
	for _, unit := range units {
		if strings.TrimSpace(unit.ID) == "" || strings.TrimSpace(unit.AgentID) == "" {
			return nil, errors.New("proposal freeze requires work-unit id and agent id")
		}
		if seenUnits[unit.ID] {
			return nil, fmt.Errorf("duplicate work-unit id: %s", unit.ID)
		}
		seenUnits[unit.ID] = true
	}

	maxConcurrency := config.MaxConcurrency
	if maxConcurrency <= 0 {
		maxConcurrency = 2
	}
	if maxConcurrency > 16 {
		maxConcurrency = 16
	}
	policy := config.Policy
	if len(policy.AllowedExtensions) == 0 && len(policy.AllowedBasenames) == 0 {
		policy = execution.DefaultSafeViewPolicy()
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	sem := make(chan struct{}, maxConcurrency)
	results := make(chan proposalResult, len(units))

	for _, unit := range units {
		unit := unit
		go func() {
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-runCtx.Done():
				results <- proposalResult{err: runCtx.Err()}
				return
			}

			runtime, err := resolve(unit)
			if err != nil {
				results <- proposalResult{err: fmt.Errorf("resolve proposal runtime for %s: %w", unit.ID, err)}
				return
			}
			if runtime == nil {
				results <- proposalResult{err: fmt.Errorf("proposal runtime for %s is nil", unit.ID)}
				return
			}

			value, err := (proposal.Generator{Runtime: runtime}).Generate(runCtx, unit)
			if err != nil {
				results <- proposalResult{err: fmt.Errorf("generate proposal for %s: %w", unit.ID, err)}
				return
			}
			declared, err := staging.ValidateDeclaredFiles(value.Files, policy)
			if err != nil {
				results <- proposalResult{err: fmt.Errorf("validate proposal scope for %s: %w", unit.ID, err)}
				return
			}
			results <- proposalResult{frozen: FrozenProposal{
				TaskID:   unit.ID,
				AgentID:  unit.AgentID,
				Proposal: cloneProposal(value),
				Declared: append([]staging.WorkspaceChange(nil), declared...),
			}}
		}()
	}

	var frozen []FrozenProposal
	var firstErr error
	for range units {
		result := <-results
		if result.err != nil {
			if firstErr == nil {
				firstErr = result.err
				cancel()
			}
			continue
		}
		frozen = append(frozen, result.frozen)
	}
	if firstErr != nil {
		return nil, firstErr
	}

	sort.Slice(frozen, func(i, j int) bool { return frozen[i].TaskID < frozen[j].TaskID })
	if err := validateFrozenOwnership(frozen); err != nil {
		return nil, err
	}
	return frozen, nil
}

func validateFrozenOwnership(values []FrozenProposal) error {
	owners := map[string]string{}
	for _, value := range values {
		for _, change := range value.Declared {
			if owner, exists := owners[change.Path]; exists {
				return fmt.Errorf(
					"proposal ownership conflict on %s between tasks %s and %s",
					change.Path,
					owner,
					value.TaskID,
				)
			}
			owners[change.Path] = value.TaskID
		}
	}
	return nil
}

func cloneProposal(value proposal.Proposal) proposal.Proposal {
	clone := value
	clone.Files = append([]proposal.FileChange(nil), value.Files...)
	clone.Risks = append([]proposal.Risk(nil), value.Risks...)
	clone.Verification = append([]string(nil), value.Verification...)
	return clone
}
