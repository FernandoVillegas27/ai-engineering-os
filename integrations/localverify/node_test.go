package localverify

import (
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
