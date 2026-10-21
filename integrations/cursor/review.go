package cursor

import (
	"context"
	"fmt"
	"strings"

	"github.com/FernandoVillegas27/ai-engineering-os/core/review"
)

type ReviewRuntime struct{ Config RunnerConfig }

func NewReviewRuntime() ReviewRuntime {
	config := defaultConfig()
	config.Model = defaultReviewModel
	return ReviewRuntime{Config: config}
}
func (ReviewRuntime) ID() string      { return "cursor-review" }

func (r ReviewRuntime) Review(ctx context.Context, workspace, task string, changedFiles []string) (review.Report, error) {
	out, err := runAgent(ctx, r.Config, workspace, "ask", reviewPrompt(task, changedFiles))
	if err != nil {
		return review.Report{}, err
	}

	var payload struct {
		Summary  string
		Findings []review.Finding
	}
	if err := decodeJSONObject(out, &payload); err != nil {
		return review.Report{}, err
	}

	report := review.Report{
		Verdict:  review.VerdictApproved,
		Summary:  payload.Summary,
		Findings: payload.Findings,
	}
	for _, finding := range report.Findings {
		switch strings.ToLower(strings.TrimSpace(finding.Severity)) {
		case "medium", "high", "critical":
			report.Verdict = review.VerdictRejected
		}
	}
	return report, nil
}

func reviewPrompt(task string, changedFiles []string) string {
	return fmt.Sprintf(`Perform an INDEPENDENT READ-ONLY quality review of this isolated staged workspace.
Do not modify files, install dependencies, use network resources, commit, or push.
Check correctness, regressions, edge cases, security, and whether the implementation satisfies the original task.
Only report concrete findings grounded in code. Medium/high/critical findings block approval.

Return EXACTLY one JSON object and no markdown:
{"Summary":"...","Findings":[{"Severity":"info|low|medium|high|critical","Title":"...","Detail":"..."}]}

Write the summary and findings in Spanish.

Original task: %s
Staged changed files: %s
`, task, strings.Join(changedFiles, ", "))
}
