package localverify

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/FernandoVillegas27/ai-engineering-os/core/verification"
)

var (
	gradleTestFailure = regexp.MustCompile(`(?m)^\s*([A-Za-z0-9_.$]+)\s+>\s+(.+?)\s+FAILED\s*$`)
	mavenTestFailure = regexp.MustCompile(`(?m)^\[ERROR\]\s+([A-Za-z0-9_.$]+)\.([^\s]+)\s+--.*<<<\s+(?:FAILURE|ERROR)!\s*$`)
	mavenCompileFailure = regexp.MustCompile(`(?m)^\[ERROR\]\s+(.+?\.java):\[(\d+),(\d+)\]\s+(.+)$`)
	javacCompileFailure = regexp.MustCompile(`(?m)^\s*(.+?\.java):(\d+):\s+error:\s+(.+)$`)
	gradleFailedSummary = regexp.MustCompile(`(?m)^(\d+) tests completed, (\d+) failed(?:, \d+ skipped)?\s*$`)
	mavenFailedSummary = regexp.MustCompile(`Failures:\s*(\d+),\s*Errors:\s*(\d+)`)
)

type structuredParse struct {
	Failures   []verification.Failure
	Confidence verification.ParseConfidence
}

func parseJavaTestOutput(stdout, stderr string, exitFailed bool, roots ...string) structuredParse {
	text := strings.ReplaceAll(stdout+"\n"+stderr, "\r\n", "\n")
	seen := map[string]bool{}
	var failures []verification.Failure
	add := func(id string, class verification.FailureClass, signatureText string) {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			return
		}
		seen[id] = true
		failures = append(failures, verification.Failure{
			ID: id, Class: class, Signature: verification.Signature(signatureText, roots...),
		})
	}

	for _, match := range gradleTestFailure.FindAllStringSubmatch(text, -1) {
		id := match[1] + "#" + strings.TrimSpace(match[2])
		add(id, verification.FailureClassTest, failureContext(text, match[0]))
	}
	for _, match := range mavenTestFailure.FindAllStringSubmatch(text, -1) {
		id := match[1] + "#" + strings.TrimSpace(match[2])
		add(id, verification.FailureClassTest, failureContext(text, match[0]))
	}
	for _, match := range mavenCompileFailure.FindAllStringSubmatch(text, -1) {
		path := normalizedFailurePath(match[1], roots...)
		id := fmt.Sprintf("%s:%s:%s", path, match[2], match[3])
		add(id, verification.FailureClassBuild, failureContext(text, match[0]))
	}
	for _, match := range javacCompileFailure.FindAllStringSubmatch(text, -1) {
		path := normalizedFailurePath(match[1], roots...)
		id := fmt.Sprintf("%s:%s", path, match[2])
		add(id, verification.FailureClassBuild, failureContext(text, match[0]))
	}

	sortFailures(failures)
	if !exitFailed {
		return structuredParse{Failures: failures, Confidence: verification.ParseComplete}
	}
	if len(failures) == 0 {
		return structuredParse{Confidence: verification.ParseFailed}
	}

	confidence := verification.ParseComplete
	if expected, ok := expectedJavaTestFailures(text); ok {
		actualTests := 0
		for _, failure := range failures {
			if failure.Class == verification.FailureClassTest || failure.Class == verification.FailureClassSubtest {
				actualTests++
			}
		}
		if expected > actualTests {
			confidence = verification.ParsePartial
		}
	}
	return structuredParse{Failures: failures, Confidence: confidence}
}

func expectedJavaTestFailures(text string) (int, bool) {
	if match := gradleFailedSummary.FindStringSubmatch(text); len(match) == 3 {
		value, err := strconv.Atoi(match[2])
		return value, err == nil
	}
	if match := mavenFailedSummary.FindStringSubmatch(text); len(match) == 3 {
		failures, err1 := strconv.Atoi(match[1])
		errorsCount, err2 := strconv.Atoi(match[2])
		return failures + errorsCount, err1 == nil && err2 == nil
	}
	return 0, false
}

func failureContext(text, marker string) string {
	index := strings.Index(text, marker)
	if index < 0 {
		return marker
	}
	end := index + len(marker)
	lines := 0
	for end < len(text) && lines < 8 {
		next := strings.IndexByte(text[end:], '\n')
		if next < 0 {
			end = len(text)
			break
		}
		end += next + 1
		lines++
	}
	return text[index:end]
}

func normalizedFailurePath(value string, roots ...string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	normalized := verification.NormalizeOutput(value, roots...)
	normalized = strings.ReplaceAll(normalized, "\\", "/")
	return normalized
}

func sortFailures(values []verification.Failure) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j].ID < values[j-1].ID; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}
