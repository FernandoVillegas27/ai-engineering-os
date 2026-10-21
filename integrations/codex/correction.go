package codex

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/quality"
	"github.com/FernandoVillegas27/ai-engineering-os/core/staging"
	"github.com/FernandoVillegas27/ai-engineering-os/integrations/commandmodel"
)

type CorrectionRuntime struct {
	Binary  string
	Model   string
	Runner  commandmodel.Runner
	Timeout time.Duration
}

func NewCorrectionRuntime() CorrectionRuntime {
	return CorrectionRuntime{
		Binary:  "codex",
		Runner:  commandmodel.OSRunner{},
		Timeout: defaultRuntimeTimeout,
	}
}

func (CorrectionRuntime) ID() string { return "codex-corrector" }

func (r CorrectionRuntime) Correct(ctx context.Context, request quality.CorrectionRequest) error {
	if strings.TrimSpace(request.Workspace) == "" {
		return fmt.Errorf("correction workspace is required")
	}
	if len(request.AllowedFiles) == 0 {
		return fmt.Errorf("correction allowed files are required")
	}

	policy := execution.DefaultSafeViewPolicy()
	allowed := map[string]bool{}
	for _, value := range request.AllowedFiles {
		clean, err := staging.ValidateWorkspacePath(value, policy)
		if err != nil {
			return fmt.Errorf("invalid correction path %q: %w", value, err)
		}
		allowed[clean] = true
	}

	before, err := staging.SnapshotWorkspace(request.Workspace, policy)
	if err != nil {
		return err
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
		return fmt.Errorf("codex command not found in PATH: %w", err)
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
		Stdin:   correctionPrompt(request),
		Dir:     request.Workspace,
		Env:     safeRuntimeEnvironment(),
	}

	timeout := r.Timeout
	if timeout <= 0 {
		timeout = defaultRuntimeTimeout
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	_, stderr, runErr := runner.Run(runCtx, invocation)
	if runErr != nil {
		if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("Codex correction timed out after %s", timeout)
		}
		if strings.TrimSpace(stderr) != "" {
			return fmt.Errorf("Codex correction failed: %w: %s", runErr, strings.TrimSpace(stderr))
		}
		return fmt.Errorf("Codex correction failed: %w", runErr)
	}

	actual, err := staging.DetectWorkspaceChanges(before, request.Workspace, policy)
	if err != nil {
		return err
	}
	if len(actual) == 0 {
		return fmt.Errorf("Codex correction produced no file changes")
	}
	for _, change := range actual {
		if !allowed[change.Path] {
			return fmt.Errorf("Codex correction changed undeclared file: %s", change.Path)
		}
	}

	return nil
}

func correctionPrompt(request quality.CorrectionRequest) string {
	var b strings.Builder
	b.WriteString("Correct the staged implementation inside this isolated workspace.\n")
	b.WriteString("You MAY modify ONLY the approved files listed below. Do not touch any other file.\n")
	b.WriteString("Do not commit, push, install dependencies, access secrets, production systems, or anything outside this workspace.\n")
	b.WriteString("Fix the concrete verification/review failures while preserving the original task intent.\n\n")
	b.WriteString("Original task: " + request.Task + "\n")
	b.WriteString("Approved files:\n")
	for _, file := range request.AllowedFiles {
		b.WriteString("- " + file + "\n")
	}
	b.WriteString("\nFailures to correct:\n")
	failure := quality.FailureSummary(request.Verification, request.Review)
	if strings.TrimSpace(failure) == "" {
		failure = "Quality gate rejected the implementation. Inspect the staged code and correct the issue without expanding scope."
	}
	b.WriteString(failure)
	b.WriteString("\n\nApply the corrections directly to this isolated workspace, then stop.\n")
	return b.String()
}

