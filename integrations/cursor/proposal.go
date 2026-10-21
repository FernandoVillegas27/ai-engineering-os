package cursor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/proposal"
	"github.com/FernandoVillegas27/ai-engineering-os/integrations/commandmodel"
)

type ProposalRuntime struct{ Config RunnerConfig }

func NewProposalRuntime() ProposalRuntime { return ProposalRuntime{Config: defaultConfig()} }
func (ProposalRuntime) ID() string        { return RuntimeID }

func (r ProposalRuntime) Propose(ctx context.Context, unit execution.WorkUnit) (proposal.Proposal, error) {
	if err := execution.ValidateReadOnlyUnit(unit); err != nil {
		return proposal.Proposal{}, fmt.Errorf("invalid Cursor proposal work unit: %w", err)
	}
	projectRoot, err := filepath.Abs(unit.ProjectRoot)
	if err != nil {
		return proposal.Proposal{}, fmt.Errorf("resolve project root: %w", err)
	}
	tempDir, err := os.MkdirTemp(filepath.Dir(projectRoot), ".ai-engineering-os-cursor-proposal-*")
	if err != nil {
		return proposal.Proposal{}, fmt.Errorf("create Cursor proposal workspace: %w", err)
	}
	defer os.RemoveAll(tempDir)

	view, err := execution.PrepareSafeReadOnlyView(projectRoot, filepath.Join(tempDir, "project"))
	if err != nil {
		return proposal.Proposal{}, err
	}

	out, err := runAgent(ctx, r.Config, view.Root, "ask", proposalPrompt(unit, view))
	if err != nil {
		return proposal.Proposal{}, err
	}
	var result proposal.Proposal
	if err := decodeJSONObject(out, &result); err != nil {
		return proposal.Proposal{}, err
	}
	return result, nil
}

func proposalPrompt(unit execution.WorkUnit, view execution.SafeViewReport) string {
	return fmt.Sprintf(`Act as the assigned AI Engineering OS agent and PROPOSE a software change without applying it.
This is a READ-ONLY sanitized project view. Do not modify files, install dependencies, use network resources, commit, or push.
Inspect only the available project evidence.

Return EXACTLY one JSON object and no markdown with this shape:
{"Summary":"...","Files":[{"Path":"relative/path","Action":"create|modify|delete","Rationale":"..."}],"Patch":"","Risks":[{"Severity":"info|low|medium|high|critical","Detail":"..."}],"Verification":["..."]}

Patch MUST be an empty string because this runtime does not use generated patches.
Use repository-relative forward-slash paths only. Write Summary, Rationale, Risks and Verification in Spanish.

Task: %s
Intent: %s
Agent: %s
Capabilities: %s
Enabled skills: %s
Sanitized files available: %d
`, unit.Task, unit.Intent, unit.AgentID, strings.Join(unit.Capabilities, ", "), strings.Join(unit.Skills, ", "), view.CopiedFiles)
}

var _ commandmodel.Runner
