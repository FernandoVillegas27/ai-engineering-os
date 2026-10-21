package codex

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/proposal"
	"github.com/FernandoVillegas27/ai-engineering-os/core/staging"
	"github.com/FernandoVillegas27/ai-engineering-os/integrations/commandmodel"
)

type StageResult struct {
	Workspace string
	Changes   []staging.WorkspaceChange
	Summary   string
}

type StageRuntime struct {
	Binary  string
	Model   string
	Runner  commandmodel.Runner
	Timeout time.Duration
}

func NewStageRuntime() StageRuntime {
	return StageRuntime{
		Binary:  "codex",
		Runner:  commandmodel.OSRunner{},
		Timeout: defaultRuntimeTimeout,
	}
}

func (r StageRuntime) Stage(ctx context.Context, unit execution.WorkUnit, p proposal.Proposal) (StageResult, error) {
	if err := execution.ValidateReadOnlyUnit(unit); err != nil {
		return StageResult{}, fmt.Errorf("invalid Codex stage work unit: %w", err)
	}
	if strings.TrimSpace(unit.ProjectRoot) == "" {
		return StageResult{}, fmt.Errorf("project root is required")
	}

	policy := execution.DefaultSafeViewPolicy()
	declared, err := staging.ValidateDeclaredFiles(p.Files, policy)
	if err != nil {
		return StageResult{}, err
	}

	projectRoot, err := filepath.Abs(unit.ProjectRoot)
	if err != nil {
		return StageResult{}, fmt.Errorf("resolve project root: %w", err)
	}
	workspace, err := os.MkdirTemp(filepath.Dir(projectRoot), ".ai-engineering-os-stage-*")
	if err != nil {
		return StageResult{}, fmt.Errorf("create isolated stage workspace: %w", err)
	}

	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(workspace)
		}
	}()

	safeView, err := execution.PrepareSafeReadOnlyView(projectRoot, workspace)
	if err != nil {
		return StageResult{}, err
	}
	if err := validateDeclaredPreconditions(workspace, declared); err != nil {
		return StageResult{}, err
	}

	before, err := staging.SnapshotWorkspace(workspace, policy)
	if err != nil {
		return StageResult{}, err
	}

	runner := r.Runner
	if runner == nil {
		runner = commandmodel.OSRunner{}
	}
	binary := strings.TrimSpace(r.Binary)
	if binary == "" {
		binary = "codex"
	}
	resolved, err := runner.LookPath(binary)
	if err != nil {
		return StageResult{}, fmt.Errorf("codex command not found in PATH: %w", err)
	}

	args := []string{
		"exec",
		"--ephemeral",
		"--skip-git-repo-check",
		"--sandbox", "workspace-write",
		"-c", `approval_policy="never"`,
	}
	if model := strings.TrimSpace(r.Model); model != "" {
		args = append(args, "--model", model)
	}
	args = append(args, "-")

	invocation := commandmodel.Invocation{
		Command: resolved,
		Args:    args,
		Stdin:   stagePrompt(unit, p, safeView, declared),
		Dir:     workspace,
		Env:     safeRuntimeEnvironment(),
	}

	timeout := r.Timeout
	if timeout <= 0 {
		timeout = defaultRuntimeTimeout
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	stdout, stderr, runErr := runner.Run(runCtx, invocation)
	if runErr != nil {
		if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
			return StageResult{}, fmt.Errorf("Codex stage timed out after %s", timeout)
		}
		if strings.TrimSpace(stderr) != "" {
			return StageResult{}, fmt.Errorf("Codex stage failed: %w: %s", runErr, strings.TrimSpace(stderr))
		}
		return StageResult{}, fmt.Errorf("Codex stage failed: %w", runErr)
	}

	actual, err := staging.DetectWorkspaceChanges(before, workspace, policy)
	if err != nil {
		return StageResult{}, err
	}
	if err := staging.EnsureChangesMatchDeclared(actual, declared); err != nil {
		return StageResult{}, err
	}

	cleanup = false
	return StageResult{
		Workspace: workspace,
		Changes:   actual,
		Summary:   strings.TrimSpace(stdout),
	}, nil
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

func stagePrompt(
	unit execution.WorkUnit,
	p proposal.Proposal,
	safeView execution.SafeViewReport,
	declared []staging.WorkspaceChange,
) string {
	var b strings.Builder
	b.WriteString("Act as the assigned AI Engineering OS agent and IMPLEMENT the approved change inside this isolated sanitized workspace.\n")
	b.WriteString("You MAY modify files in this workspace, but ONLY the explicitly declared files listed below.\n")
	b.WriteString("Do not access or modify anything outside the current workspace. Do not access secrets, credentials, production systems, or network resources.\n")
	b.WriteString("Do not commit, push, install dependencies, or alter Git configuration. Do not create hidden credential/config files.\n")
	b.WriteString("Do not run long-lived services. Make only the requested source/test changes.\n")
	b.WriteString("The original repository is not this directory. Treat this workspace as disposable staging.\n\n")
	b.WriteString("Task: " + unit.Task + "\n")
	b.WriteString("Agent: " + unit.AgentID + "\n")
	b.WriteString("Capabilities: " + strings.Join(unit.Capabilities, ", ") + "\n")
	b.WriteString("Proposal summary: " + p.Summary + "\n")
	b.WriteString(fmt.Sprintf("Sanitized files available: %d\n", safeView.CopiedFiles))
	b.WriteString("Approved file changes:\n")
	for _, change := range declared {
		b.WriteString(fmt.Sprintf("- [%s] %s\n", change.Action, change.Path))
	}
	b.WriteString("\nImplement the task now. When finished, stop. Do not describe a patch; write the files directly in this isolated workspace.\n")
	return b.String()
}
