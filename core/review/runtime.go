package review

import "context"

type Verdict string

const (
	VerdictApproved Verdict = "approved"
	VerdictRejected Verdict = "rejected"
)

type Finding struct {
	Severity string
	Title    string
	Detail   string
}

type Report struct {
	Verdict  Verdict
	Summary  string
	Findings []Finding
}

type Runtime interface {
	ID() string
	Review(ctx context.Context, workspace string, task string, changedFiles []string) (Report, error)
}
