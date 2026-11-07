package localverify

import (
	"os"
	"strings"
	"testing"

	"github.com/FernandoVillegas27/ai-engineering-os/core/verification"
)


func parseJUnitFixture(t *testing.T, data string, exitFailed bool, roots ...string) structuredParse {
	t.Helper()
	file, err := os.CreateTemp(t.TempDir(), "pytest-*.xml")
	if err != nil {
		t.Fatal(err)
	}
	path := file.Name()
	if _, err := file.WriteString(data); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return parsePytestJUnitFile(path, exitFailed, roots...)
}

const pytestJUnitFixture = `<?xml version="1.0" encoding="utf-8"?>
<testsuites tests="3" failures="1" errors="1" skipped="0">
  <testsuite name="pytest" errors="1" failures="1" skipped="0" tests="3">
    <testcase classname="tests.test_math" name="test_adds" file="C:/workspace/tests/test_math.py" line="10">
      <failure message="assert 4 == 3" type="AssertionError">C:/workspace/tests/test_math.py:12: AssertionError</failure>
    </testcase>
    <testcase classname="tests.test_math" name="test_fixture" file="C:/workspace/tests/test_math.py" line="20">
      <error message="fixture setup failed" type="RuntimeError">C:/workspace/tests/conftest.py:8: RuntimeError</error>
    </testcase>
    <testcase classname="tests.test_math" name="test_passes" file="C:/workspace/tests/test_math.py" line="30"/>
  </testsuite>
</testsuites>`

func TestParsePytestJUnitNormalizesFailures(t *testing.T) {
	got := parseJUnitFixture(t, pytestJUnitFixture, true, "C:/workspace")
	if got.Confidence != verification.ParseComplete || len(got.Failures) != 2 {
		t.Fatalf("got=%#v", got)
	}
	if first := failureByID(t, got.Failures, "tests/test_math.py::test_adds"); first.Class != verification.FailureClassTest {
		t.Fatalf("first=%#v", first)
	}
	failureByID(t, got.Failures, "tests/test_math.py::test_fixture")
}

func TestParsePytestJUnitSignaturesIgnoreWorkspaceLocation(t *testing.T) {
	first := parseJUnitFixture(t, pytestJUnitFixture, true, "C:/workspace")
	movedFixture := strings.ReplaceAll(pytestJUnitFixture, "C:/workspace", "D:/other/stage")
	second := parseJUnitFixture(t, movedFixture, true, "D:/other/stage")
	for _, failure := range first.Failures {
		other := failureByID(t, second.Failures, failure.ID)
		if failure.Signature != other.Signature {
			t.Fatalf("%s signature changed: %s != %s", failure.ID, failure.Signature, other.Signature)
		}
	}
}

func TestParsePytestJUnitDetectsIncompleteFailureEvidence(t *testing.T) {
	partialFixture := strings.Replace(pytestJUnitFixture, `failures="1" errors="1"`, `failures="2" errors="1"`, 1)
	got := parseJUnitFixture(t, partialFixture, true, "C:/workspace")
	if got.Confidence != verification.ParsePartial || len(got.Failures) != 2 {
		t.Fatalf("got=%#v", got)
	}
}

func TestParsePytestJUnitPassingReportIsComplete(t *testing.T) {
	fixture := `<testsuites tests="1" failures="0" errors="0"><testsuite tests="1" failures="0" errors="0"><testcase classname="x" name="test_ok" file="tests/test_ok.py"/></testsuite></testsuites>`
	got := parseJUnitFixture(t, fixture, false)
	if got.Confidence != verification.ParseComplete || len(got.Failures) != 0 {
		t.Fatalf("got=%#v", got)
	}
}
