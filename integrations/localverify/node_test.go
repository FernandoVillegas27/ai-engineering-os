package localverify

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FernandoVillegas27/ai-engineering-os/core/verification"
)

const nodeJSONFixture = `{
  "numFailedTestSuites": 1,
  "numFailedTests": 1,
  "testResults": [
    {
      "name": "C:/workspace/src/calc.test.ts",
      "assertionResults": [
        {
          "ancestorTitles": ["calc"],
          "title": "adds",
          "fullName": "calc adds",
          "status": "failed",
          "failureMessages": ["AssertionError: expected 4 to be 3 at C:/workspace/src/calc.test.ts:10:2"]
        },
        {
          "ancestorTitles": ["calc"],
          "title": "subtracts",
          "fullName": "calc subtracts",
          "status": "passed",
          "failureMessages": []
        }
      ]
    }
  ]
}`

func TestParseNodeJSONFailure(t *testing.T) {
	got := parseNodeTestJSON(nodeJSONFixture, "", true, "C:/workspace")
	if got.Confidence != verification.ParseComplete || len(got.Failures) != 1 {
		t.Fatalf("got=%#v", got)
	}
	failure := got.Failures[0]
	if failure.ID != "src/calc.test.ts#calc adds" || failure.Class != verification.FailureClassTest {
		t.Fatalf("failure=%#v", failure)
	}
}

func TestParseNodeJSONCanReadReporterFromStderr(t *testing.T) {
	got := parseNodeTestJSON("", "npm noise\n"+nodeJSONFixture+"\n", true, "C:/workspace")
	if got.Confidence != verification.ParseComplete || len(got.Failures) != 1 {
		t.Fatalf("got=%#v", got)
	}
}

func TestParseNodeJSONSignatureIgnoresWorkspace(t *testing.T) {
	first := parseNodeTestJSON(nodeJSONFixture, "", true, "C:/workspace")
	moved := strings.ReplaceAll(nodeJSONFixture, "C:/workspace", "D:/other/stage")
	second := parseNodeTestJSON(moved, "", true, "D:/other/stage")
	if first.Failures[0].Signature != second.Failures[0].Signature {
		t.Fatalf("signature changed across workspaces: %s != %s", first.Failures[0].Signature, second.Failures[0].Signature)
	}
}

func TestParseNodeJSONDetectsMissingFailures(t *testing.T) {
	partial := strings.Replace(nodeJSONFixture, `"numFailedTests": 1`, `"numFailedTests": 2`, 1)
	got := parseNodeTestJSON(partial, "", true, "C:/workspace")
	if got.Confidence != verification.ParsePartial {
		t.Fatalf("got=%#v", got)
	}

	failed := parseNodeTestJSON("not json", "", true)
	if failed.Confidence != verification.ParseFailed {
		t.Fatalf("failed=%#v", failed)
	}
}


func TestParseNodeJSONFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.json")
	if err := os.WriteFile(path, []byte(nodeJSONFixture), 0o600); err != nil {
		t.Fatal(err)
	}
	got := parseNodeTestJSONFile(path, true, "C:/workspace")
	if got.Confidence != verification.ParseComplete || len(got.Failures) != 1 {
		t.Fatalf("got=%#v", got)
	}
	failureByID(t, got.Failures, "src/calc.test.ts#calc adds")
}

func TestParseNodeJSONFileMissingFailsClosed(t *testing.T) {
	got := parseNodeTestJSONFile(filepath.Join(t.TempDir(), "missing.json"), true)
	if got.Confidence != verification.ParseFailed {
		t.Fatalf("got=%#v", got)
	}
}


func TestParseNodeJSONRejectsDuplicateFailureIdentity(t *testing.T) {
	// Two failed assertions with the same stable ID cannot be silently
	// deduplicated into complete evidence.
	duplicate := strings.Replace(nodeJSONFixture,
		`"numFailedTests": 1`, `"numFailedTests": 2`, 1)
	needle := `{
          "ancestorTitles": ["calc"],
          "title": "subtracts",
          "fullName": "calc subtracts",
          "status": "passed",
          "failureMessages": []
        }`
	replacement := `{
          "ancestorTitles": ["calc"],
          "title": "adds",
          "fullName": "calc adds",
          "status": "failed",
          "failureMessages": ["different assertion"]
        }`
	duplicate = strings.Replace(duplicate, needle, replacement, 1)
	got := parseNodeTestJSON(duplicate, "", true, "C:/workspace")
	if got.Confidence != verification.ParsePartial || len(got.Failures) != 1 {
		t.Fatalf("duplicate failure ID must be partial, got=%#v", got)
	}
}

func TestParseNodeJSONRejectsConflictingCounts(t *testing.T) {
	for _, count := range []string{"0", "2"} {
		report := strings.Replace(nodeJSONFixture, `"numFailedTests": 1`, `"numFailedTests": `+count, 1)
		got := parseNodeTestJSON(report, "", true, "C:/workspace")
		if got.Confidence != verification.ParsePartial || len(got.Failures) != 1 {
			t.Fatalf("count=%s must be partial, got=%#v", count, got)
		}
	}
}

func TestParseNodeJSONRejectsFailedAssertionsOnSuccessExit(t *testing.T) {
	got := parseNodeTestJSON(nodeJSONFixture, "", false, "C:/workspace")
	if got.Confidence != verification.ParsePartial || len(got.Failures) != 1 {
		t.Fatalf("successful exit with reported failures must be partial, got=%#v", got)
	}
}

func TestParseNodeJSONValidPassingReportRemainsComplete(t *testing.T) {
	passing := strings.Replace(nodeJSONFixture, `"numFailedTests": 1`, `"numFailedTests": 0`, 1)
	passing = strings.Replace(passing, `"status": "failed"`, `"status": "passed"`, 1)
	got := parseNodeTestJSON(passing, "", false, "C:/workspace")
	if got.Confidence != verification.ParseComplete || len(got.Failures) != 0 {
		t.Fatalf("valid passing report must remain complete, got=%#v", got)
	}
}
