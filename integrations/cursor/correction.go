package cursor

import (
	"context"
	"fmt"
	"strings"

	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/quality"
	"github.com/FernandoVillegas27/ai-engineering-os/core/staging"
)

type CorrectionRuntime struct{ Config RunnerConfig }

func NewCorrectionRuntime() CorrectionRuntime { return CorrectionRuntime{Config: defaultConfig()} }
func (CorrectionRuntime) ID() string           { return "cursor-corrector" }

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
	if _, err := runAgent(ctx, r.Config, request.Workspace, "", correctionPrompt(request)); err != nil {
		return err
	}

	actual, err := staging.DetectWorkspaceChanges(before, request.Workspace, policy)
	if err != nil {
		return err
	}
	if len(actual) == 0 {
		return fmt.Errorf("Cursor correction produced no file changes")
	}
	for _, change := range actual {
		if !allowed[change.Path] {
			return fmt.Errorf("Cursor correction changed undeclared file: %s", change.Path)
		}
	}
	return nil
}

func correctionPrompt(request quality.CorrectionRequest) string {
	var b strings.Builder
	b.WriteString("Correct the staged implementation directly inside this isolated workspace.\n")
	b.WriteString("Modify ONLY the approved files listed below. Do not access anything outside this workspace.\n")
	b.WriteString("Do not install dependencies, use network resources, access secrets, commit, push, or alter Git configuration.\n")
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
	b.WriteString("\n\nApply corrections directly and stop.\n")
	return b.String()
}
