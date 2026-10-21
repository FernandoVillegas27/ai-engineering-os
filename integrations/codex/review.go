package codex

import (
	"context"
	"fmt"
	"strings"

	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/orchestration"
	"github.com/FernandoVillegas27/ai-engineering-os/core/review"
)

type ReviewRuntime struct {
	Execution ExecutionRuntime
}

func NewReviewRuntime() ReviewRuntime {
	return ReviewRuntime{Execution: NewExecutionRuntime()}
}

func (ReviewRuntime) ID() string { return "codex-review" }

func (r ReviewRuntime) Review(ctx context.Context, workspace string, task string, changedFiles []string) (review.Report, error) {
	executionRuntime := r.Execution
	if strings.TrimSpace(executionRuntime.Binary) == "" && executionRuntime.Runner == nil {
		executionRuntime = NewExecutionRuntime()
	}

	unit := execution.WorkUnit{
		ID:          "quality-review",
		Task:        reviewTask(task, changedFiles),
		ProjectRoot: workspace,
		Intent:      orchestration.IntentReview,
		AgentID:     "reviewer",
		Capabilities: []string{
			"code-review",
			"security-review",
		},
		Mode: execution.ModeReadOnly,
		Permissions: execution.PermissionEnvelope{
			SourceCode: "read",
			Tests:      "read",
			GitCommit:  "deny",
			GitPush:    "deny",
			Secrets:    "deny",
			Production: "deny",
		},
	}

	analysis, err := executionRuntime.Analyze(ctx, unit)
	if err != nil {
		return review.Report{}, fmt.Errorf("independent Codex review: %w", err)
	}

	report := review.Report{
		Verdict: review.VerdictApproved,
		Summary: analysis.Summary,
	}
	for _, finding := range analysis.Findings {
		report.Findings = append(report.Findings, review.Finding{
			Severity: finding.Severity,
			Title:    finding.Title,
			Detail:   finding.Detail,
		})
		switch strings.ToLower(finding.Severity) {
		case "medium", "high", "critical":
			report.Verdict = review.VerdictRejected
		}
	}
	return report, nil
}

func reviewTask(task string, changedFiles []string) string {
	var b strings.Builder
	b.WriteString("Perform an independent quality review of the staged implementation. ")
	b.WriteString("Check correctness, regressions, edge cases, security, and whether the implementation satisfies the task. ")
	b.WriteString("Only report concrete findings grounded in the staged code. Medium/high/critical findings block approval.\n\n")
	b.WriteString("Original task: ")
	b.WriteString(task)
	if len(changedFiles) > 0 {
		b.WriteString("\nStaged changed files: ")
		b.WriteString(strings.Join(changedFiles, ", "))
	}
	return b.String()
}
