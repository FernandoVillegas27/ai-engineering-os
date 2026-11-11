package localverify

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/FernandoVillegas27/ai-engineering-os/core/verification"
)

type nodeJSONReport struct {
	NumFailedTests      int              `json:"numFailedTests"`
	NumFailedTestSuites int              `json:"numFailedTestSuites"`
	TestResults         []nodeTestResult `json:"testResults"`
}

type nodeTestResult struct {
	Name             string          `json:"name"`
	AssertionResults []nodeAssertion `json:"assertionResults"`
}

type nodeAssertion struct {
	AncestorTitles  []string `json:"ancestorTitles"`
	Title           string   `json:"title"`
	FullName        string   `json:"fullName"`
	Status          string   `json:"status"`
	FailureMessages []string `json:"failureMessages"`
}

func parseNodeTestJSONFile(path string, exitFailed bool, roots ...string) structuredParse {
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
	return parseNodeTestJSON(string(data), "", exitFailed, roots...)
}

func parseNodeTestJSON(stdout, stderr string, exitFailed bool, roots ...string) structuredParse {
	raw, ok := extractNodeJSON(stdout)
	if !ok {
		raw, ok = extractNodeJSON(stderr)
	}
	if !ok {
		if exitFailed {
			return structuredParse{Confidence: verification.ParseFailed}
		}
		return structuredParse{Confidence: verification.ParseNotAttempted}
	}
	var report nodeJSONReport
	if err := json.Unmarshal(raw, &report); err != nil {
		if exitFailed {
			return structuredParse{Confidence: verification.ParseFailed}
		}
		return structuredParse{Confidence: verification.ParsePartial}
	}
	// Require actual reporter structure. Without these keys, Go's zero-value
	// decoding can make an arbitrary JSON object look like a passing test run.
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(raw, &envelope); err != nil ||
		!nodeRequiredJSONFields(envelope) || report.NumFailedTests < 0 ||
		report.NumFailedTestSuites < 0 || len(report.TestResults) == 0 {
		return structuredParse{Confidence: verification.ParsePartial}
	}

	var failures []verification.Failure
	seen := map[string]bool{}
	duplicateFailureIDs := false
	invalidAssertionStatus := false
	for _, suite := range report.TestResults {
		suiteID := normalizedNodeSuite(suite.Name, roots...)
		for _, assertion := range suite.AssertionResults {
			switch strings.ToLower(strings.TrimSpace(assertion.Status)) {
			case "passed", "pending", "skipped", "todo", "disabled":
				continue
			case "failed":
				// Extract and retain the failure below.
			default:
				invalidAssertionStatus = true
				continue
			}
			name := strings.TrimSpace(assertion.FullName)
			if name == "" {
				parts := append([]string(nil), assertion.AncestorTitles...)
				parts = append(parts, assertion.Title)
				name = strings.Join(parts, " > ")
			}
			if name == "" {
				name = "unknown-test"
			}
			id := suiteID + "#" + name
			if seen[id] {
				// Baseline comparison indexes by failure ID. Discarding a
				// duplicate here would conceal conflicting test evidence.
				duplicateFailureIDs = true
				continue
			}
			seen[id] = true
			detail := strings.Join(assertion.FailureMessages, "\n")
			if strings.TrimSpace(detail) == "" {
				detail = id
			}
			failures = append(failures, verification.Failure{
				ID: id,
				Class: verification.FailureClassTest,
				Signature: verification.Signature(detail, roots...),
			})
		}
	}
	sortFailures(failures)

	// A report with duplicate failure identities or conflicting totals is
	// incomplete even when an individual failure can be identified. In
	// particular, do not treat a successful process exit with failures as
	// complete evidence for baseline-aware Quality.
	if duplicateFailureIDs || invalidAssertionStatus || report.NumFailedTests != len(failures) ||
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

func extractNodeJSON(value string) ([]byte, bool) {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\r\n", "\n"))
	if value == "" {
		return nil, false
	}
	if json.Valid([]byte(value)) && strings.Contains(value, `"testResults"`) {
		return []byte(value), true
	}
	start := strings.Index(value, "{")
	for start >= 0 {
		for end := strings.LastIndex(value, "}"); end > start; end = strings.LastIndex(value[:end], "}") {
			candidate := strings.TrimSpace(value[start : end+1])
			if strings.Contains(candidate, `"testResults"`) && json.Valid([]byte(candidate)) {
				return []byte(candidate), true
			}
		}
		next := strings.Index(value[start+1:], "{")
		if next < 0 {
			break
		}
		start += next + 1
	}
	return nil, false
}

func normalizedNodeSuite(value string, roots ...string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "node-suite"
	}
	normalized := verification.NormalizeOutput(value, roots...)
	normalized = strings.ReplaceAll(normalized, "\\", "/")
	if strings.HasPrefix(normalized, "<workspace>/") {
		normalized = strings.TrimPrefix(normalized, "<workspace>/")
	}
	return filepath.ToSlash(normalized)
}

// nodeRequiredJSONFields keeps missing required fields distinct from valid
// zero counts. The caller must never infer successful verification from a
// JSON object that merely happens to contain a testResults property.
func nodeRequiredJSONFields(envelope map[string]json.RawMessage) bool {
	if envelope == nil {
		return false
	}
	for _, name := range []string{"numFailedTests", "testResults"} {
		value, ok := envelope[name]
		if !ok || len(value) == 0 || string(value) == "null" {
			return false
		}
	}
	return true
}
