package localverify

import (
	"encoding/xml"
	"os"
	"strings"

	"github.com/FernandoVillegas27/ai-engineering-os/core/verification"
)

type junitReport struct {
	XMLName  xml.Name       `xml:""`
	Tests    *int           `xml:"tests,attr"`
	Failures *int           `xml:"failures,attr"`
	Errors   *int           `xml:"errors,attr"`
	Suites   []junitReport  `xml:"testsuite"`
	Cases    []junitTestCase `xml:"testcase"`
}

type junitTestCase struct {
	Classname string         `xml:"classname,attr"`
	Name      string         `xml:"name,attr"`
	File      string         `xml:"file,attr"`
	Line      string         `xml:"line,attr"`
	Failure   *junitProblem  `xml:"failure"`
	Error     *junitProblem  `xml:"error"`
}

type junitProblem struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Body    string `xml:",chardata"`
}

func parsePytestJUnitFile(path string, exitFailed bool, roots ...string) structuredParse {
	if strings.TrimSpace(path) == "" {
		if exitFailed {
			return structuredParse{Confidence: verification.ParseFailed}
		}
		return structuredParse{Confidence: verification.ParseNotAttempted}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if exitFailed {
			return structuredParse{Confidence: verification.ParseFailed}
		}
		return structuredParse{Confidence: verification.ParsePartial}
	}
	var report junitReport
	if err := xml.Unmarshal(data, &report); err != nil ||
		(report.XMLName.Local != "testsuite" && report.XMLName.Local != "testsuites") {
		if exitFailed {
			return structuredParse{Confidence: verification.ParseFailed}
		}
		return structuredParse{Confidence: verification.ParsePartial}
	}

	var failures []verification.Failure
	collectJUnitFailures(report, &failures, roots...)
	sortFailures(failures)

	// A successful exit with reported failures or an inconsistent count is not
	// trustworthy evidence. Duplicate IDs are similarly ambiguous: the
	// baseline comparison indexes by ID and must never silently drop entries.
	expected, known := junitExpectedFailures(report)
	if junitDuplicateFailureIDs(failures) || junitInvalidCounts(report) || (known && expected != len(failures)) ||
		(!exitFailed && len(failures) > 0) {
		return structuredParse{Failures: failures, Confidence: verification.ParsePartial}
	}
	if !exitFailed {
		return structuredParse{Failures: failures, Confidence: verification.ParseComplete}
	}
	if len(failures) == 0 {
		return structuredParse{Confidence: verification.ParseFailed}
	}
	return structuredParse{Failures: failures, Confidence: verification.ParseComplete}
}

func collectJUnitFailures(report junitReport, failures *[]verification.Failure, roots ...string) {
	for _, testCase := range report.Cases {
		problem := testCase.Failure
		if problem == nil {
			problem = testCase.Error
		}
		if problem == nil {
			continue
		}

		id := pytestCaseID(testCase, roots...)
		detail := strings.TrimSpace(strings.Join([]string{
			problem.Type,
			problem.Message,
			problem.Body,
		}, "\n"))
		if detail == "" {
			detail = id
		}
		*failures = append(*failures, verification.Failure{
			ID:        id,
			Class:     verification.FailureClassTest,
			Signature: verification.Signature(detail, roots...),
		})
	}
	for _, suite := range report.Suites {
		collectJUnitFailures(suite, failures, roots...)
	}
}

func pytestCaseID(testCase junitTestCase, roots ...string) string {
	name := strings.TrimSpace(testCase.Name)
	if name == "" {
		name = "unknown-test"
	}
	file := normalizedFailurePath(testCase.File, roots...)
	file = strings.TrimPrefix(file, "<workspace>/")
	if file != "" {
		return file + "::" + name
	}
	classname := strings.TrimSpace(testCase.Classname)
	if classname != "" {
		return classname + "::" + name
	}
	return name
}

func junitExpectedFailures(report junitReport) (int, bool) {
	if report.Failures != nil || report.Errors != nil {
		failures := 0
		errorsCount := 0
		if report.Failures != nil {
			failures = *report.Failures
		}
		if report.Errors != nil {
			errorsCount = *report.Errors
		}
		return failures + errorsCount, true
	}
	total := 0
	known := false
	for _, suite := range report.Suites {
		value, ok := junitExpectedFailures(suite)
		if ok {
			total += value
			known = true
		}
	}
	return total, known
}


func junitDuplicateFailureIDs(failures []verification.Failure) bool {
	seen := make(map[string]bool, len(failures))
	for _, failure := range failures {
		if failure.ID == "" || seen[failure.ID] {
			return true
		}
		seen[failure.ID] = true
	}
	return false
}

// junitInvalidCounts rejects contradictory nested totals rather than trusting a
// parent count that happens to match the collected failure identities.
func junitInvalidCounts(report junitReport) bool {
	for _, count := range []*int{report.Tests, report.Failures, report.Errors} {
		if count != nil && *count < 0 {
			return true
		}
	}
	if report.Tests != nil && report.Failures != nil && report.Errors != nil &&
		*report.Failures+*report.Errors > *report.Tests {
		return true
	}
	for _, suite := range report.Suites {
		if junitInvalidCounts(suite) {
			return true
		}
	}
	if len(report.Suites) == 0 || len(report.Cases) != 0 {
		return false
	}
	// Only compare totals when every child declares that dimension.
	for _, dimension := range []func(junitReport) *int{
		func(r junitReport) *int { return r.Tests },
		func(r junitReport) *int { return r.Failures },
		func(r junitReport) *int { return r.Errors },
	} {
		parent := dimension(report)
		if parent == nil {
			continue
		}
		total := 0
		complete := true
		for _, suite := range report.Suites {
			child := dimension(suite)
			if child == nil {
				complete = false
				break
			}
			total += *child
		}
		if complete && total != *parent {
			return true
		}
	}
	return false
}
