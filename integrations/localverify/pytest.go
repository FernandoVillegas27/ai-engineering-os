package localverify

import (
	"encoding/xml"
	"fmt"
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
	if err := xml.Unmarshal(data, &report); err != nil {
		if exitFailed {
			return structuredParse{Confidence: verification.ParseFailed}
		}
		return structuredParse{Confidence: verification.ParsePartial}
	}

	var failures []verification.Failure
	collectJUnitFailures(report, &failures, roots...)
	sortFailures(failures)

	if !exitFailed {
		return structuredParse{Failures: failures, Confidence: verification.ParseComplete}
	}
	if len(failures) == 0 {
		return structuredParse{Confidence: verification.ParseFailed}
	}

	expected, known := junitExpectedFailures(report)
	if known && expected > len(failures) {
		return structuredParse{Failures: failures, Confidence: verification.ParsePartial}
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

func parseJUnitFixture(data string, exitFailed bool, roots ...string) (structuredParse, error) {
	file, err := os.CreateTemp("", "ai-engineering-os-pytest-*.xml")
	if err != nil {
		return structuredParse{}, fmt.Errorf("create junit fixture: %w", err)
	}
	path := file.Name()
	defer os.Remove(path)
	if _, err := file.WriteString(data); err != nil {
		_ = file.Close()
		return structuredParse{}, err
	}
	if err := file.Close(); err != nil {
		return structuredParse{}, err
	}
	return parsePytestJUnitFile(path, exitFailed, roots...), nil
}
