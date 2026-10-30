package quality

import (
	"fmt"
	"sort"

	"github.com/FernandoVillegas27/ai-engineering-os/core/verification"
)

// FailureRef locates one failure inside a named check.
type FailureRef struct {
	Check   string
	Failure verification.Failure
}

// ChangedFailure is a failure present before and after whose class or signature differs.
type ChangedFailure struct {
	Check  string
	Before verification.Failure
	After  verification.Failure
}

// Unclassified explains why a check could not be compared reliably.
type Unclassified struct {
	Check  string
	Reason string
}

// Comparison classifies failures between a baseline and a later verification.
// It is descriptive only: "Preexisting" does not mean harmless, and no approval
// decision may be derived from it without an explicit policy.
type Comparison struct {
	Preexisting  []FailureRef
	Regressions  []FailureRef
	Resolved     []FailureRef
	ChangedCause []ChangedFailure
	Unclassified []Unclassified
	// Blocking lists every compile failure present after the change. These are
	// independent of the categories above: no tolerance policy may accept them,
	// even when the same error already existed in the baseline.
	Blocking []FailureRef
}

// Compare matches checks by name and failures by ID. Anything it cannot prove,
// including partial parses and infrastructure failures, is reported as Unclassified.
func Compare(baseline, after verification.Report) Comparison {
	base := indexChecks(baseline)
	next := indexChecks(after)

	names := map[string]bool{}
	for name := range base {
		names[name] = true
	}
	for name := range next {
		names[name] = true
	}
	ordered := make([]string, 0, len(names))
	for name := range names {
		ordered = append(ordered, name)
	}
	sort.Strings(ordered)

	var result Comparison
	for _, name := range ordered {
		before, hasBefore := base[name]
		now, hasNow := next[name]

		unclassify := func(reason string) {
			result.Unclassified = append(result.Unclassified, Unclassified{Check: name, Reason: reason})
		}

		switch {
		case !hasNow:
			unclassify("check is absent after the change")
		case now.Passed:
			if hasBefore && !before.Passed && usable(before) {
				for _, failure := range sortedFailures(before.Failures) {
					result.Resolved = append(result.Resolved, FailureRef{Check: name, Failure: failure})
				}
			}
		case now.FailureKind == verification.FailureInfrastructure:
			unclassify("infrastructure failure after the change")
		case !usable(now):
			unclassify(fmt.Sprintf("failure output after the change is not fully understood (confidence %q)", now.FailureParse))
		case !hasBefore || before.Passed:
			for _, failure := range sortedFailures(now.Failures) {
				result.Regressions = append(result.Regressions, FailureRef{Check: name, Failure: failure})
			}
		case before.FailureKind == verification.FailureInfrastructure:
			unclassify("infrastructure failure in the baseline")
		case !usable(before):
			unclassify(fmt.Sprintf("baseline failure output is not fully understood (confidence %q)", before.FailureParse))
		default:
			compareFailures(name, before.Failures, now.Failures, &result)
		}
	}

	for _, name := range ordered {
		now, hasNow := next[name]
		if !hasNow || now.Passed {
			continue
		}
		for _, failure := range sortedFailures(now.Failures) {
			if failure.Class == verification.FailureClassBuild {
				result.Blocking = append(result.Blocking, FailureRef{Check: name, Failure: failure})
			}
		}
	}
	return result
}

func compareFailures(check string, before, after []verification.Failure, result *Comparison) {
	beforeByID := map[string]verification.Failure{}
	for _, failure := range before {
		beforeByID[failure.ID] = failure
	}
	afterByID := map[string]verification.Failure{}
	for _, failure := range after {
		afterByID[failure.ID] = failure
	}

	for _, failure := range sortedFailures(after) {
		previous, existed := beforeByID[failure.ID]
		switch {
		case !existed:
			result.Regressions = append(result.Regressions, FailureRef{Check: check, Failure: failure})
		case previous.Class != failure.Class || previous.Signature != failure.Signature:
			result.ChangedCause = append(result.ChangedCause, ChangedFailure{Check: check, Before: previous, After: failure})
		default:
			result.Preexisting = append(result.Preexisting, FailureRef{Check: check, Failure: failure})
		}
	}
	for _, failure := range sortedFailures(before) {
		if _, stillFailing := afterByID[failure.ID]; !stillFailing {
			result.Resolved = append(result.Resolved, FailureRef{Check: check, Failure: failure})
		}
	}
}

// usable reports whether a failed check carries a complete, unambiguous failure list.
func usable(check verification.Check) bool {
	if check.FailureParse != verification.ParseComplete || len(check.Failures) == 0 {
		return false
	}
	seen := map[string]bool{}
	for _, failure := range check.Failures {
		if failure.ID == "" || seen[failure.ID] {
			return false
		}
		seen[failure.ID] = true
	}
	return true
}

func indexChecks(report verification.Report) map[string]verification.Check {
	checks := map[string]verification.Check{}
	for _, check := range report.Checks {
		if _, exists := checks[check.Name]; !exists {
			checks[check.Name] = check
		}
	}
	return checks
}

func sortedFailures(failures []verification.Failure) []verification.Failure {
	out := append([]verification.Failure(nil), failures...)
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
