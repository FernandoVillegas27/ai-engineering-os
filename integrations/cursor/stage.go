package cursor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/proposal"
	"github.com/FernandoVillegas27/ai-engineering-os/core/staging"
)

type StageRuntime struct{ Config RunnerConfig }

func NewStageRuntime() StageRuntime { return StageRuntime{Config: defaultConfig()} }
func (StageRuntime) ID() string     { return "cursor-stage" }

func (r StageRuntime) Stage(ctx context.Context, unit execution.WorkUnit, p proposal.Proposal) (staging.StageResult, error) {
	if err := execution.ValidateReadOnlyUnit(unit); err != nil {
		return staging.StageResult{}, fmt.Errorf("invalid Cursor stage work unit: %w", err)
	}
	policy := execution.DefaultSafeViewPolicy()
	declared, err := staging.ValidateDeclaredFiles(p.Files, policy)
	if err != nil {
		return staging.StageResult{}, err
	}

	projectRoot, err := filepath.Abs(unit.ProjectRoot)
	if err != nil {
		return staging.StageResult{}, fmt.Errorf("resolve project root: %w", err)
	}
	workspace, err := os.MkdirTemp(filepath.Dir(projectRoot), ".ai-engineering-os-stage-*")
	if err != nil {
		return staging.StageResult{}, fmt.Errorf("create Cursor stage workspace: %w", err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(workspace)
		}
	}()

	view, err := execution.PrepareSafeReadOnlyView(projectRoot, workspace)
	if err != nil {
		return staging.StageResult{}, err
	}
	if err := validateDeclaredPreconditions(workspace, declared); err != nil {
		return staging.StageResult{}, err
	}
	before, err := staging.SnapshotWorkspace(workspace, policy)
	if err != nil {
		return staging.StageResult{}, err
	}

	summary, err := runAgent(ctx, r.Config, workspace, "", stagePrompt(unit, p, view, declared))
	if err != nil {
		return staging.StageResult{}, err
	}
	actual, err := staging.DetectWorkspaceChanges(before, workspace, policy)
	if err != nil {
		return staging.StageResult{}, err
	}
	if err := staging.EnsureChangesMatchDeclared(actual, declared); err != nil {
		return staging.StageResult{}, err
	}

	cleanup = false
	return staging.StageResult{Workspace: workspace, Changes: actual, Summary: summary}, nil
}

func validateDeclaredPreconditions(workspace string, declared []staging.WorkspaceChange) error {
	for _, change := range declared {
		target := filepath.Join(workspace, filepath.FromSlash(change.Path))
		_, err := os.Stat(target)
		switch change.Action {
		case "create":
			if err == nil {
				return fmt.Errorf("declared create target already exists: %s", change.Path)
			}
			if !os.IsNotExist(err) {
				return err
			}
		case "modify", "delete":
			if err != nil {
				return fmt.Errorf("declared %s target unavailable in sanitized workspace %s: %w", change.Action, change.Path, err)
			}
		}
	}
	return nil
}

func stagePrompt(unit execution.WorkUnit, p proposal.Proposal, view execution.SafeViewReport, declared []staging.WorkspaceChange) string {
	var b strings.Builder
	b.WriteString("IMPLEMENT the approved change directly inside this isolated disposable workspace.\n")
	b.WriteString("Modify ONLY the approved files below. Do not access anything outside this workspace.\n")
	b.WriteString("Do not install dependencies, use network resources, access secrets, commit, push, or alter Git configuration.\n")
	b.WriteString("Do not create hidden configuration files. Stop after writing the requested files.\n\n")
	b.WriteString("Task: " + unit.Task + "\n")
	b.WriteString("Agent: " + unit.AgentID + "\n")
	b.WriteString("Capabilities: " + strings.Join(unit.Capabilities, ", ") + "\n")
	b.WriteString("Proposal summary: " + p.Summary + "\n")
	b.WriteString(fmt.Sprintf("Sanitized files available: %d\n", view.CopiedFiles))
	b.WriteString("Approved file changes:\n")
	for _, change := range declared {
		b.WriteString(fmt.Sprintf("- [%s] %s\n", change.Action, change.Path))
	}
	return b.String()
}
