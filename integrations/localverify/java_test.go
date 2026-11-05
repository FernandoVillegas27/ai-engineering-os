package localverify

import (
	"strings"
	"testing"

	"github.com/FernandoVillegas27/ai-engineering-os/core/verification"
)

const gradleFailureFixture = `
com.example.CalculatorTest > addsNumbers FAILED
    org.opentest4j.AssertionFailedError: expected: <3> but was: <4>
        at app//com.example.CalculatorTest.addsNumbers(CalculatorTest.java:18)

3 tests completed, 1 failed
FAILURE: Build failed with an exception.
`

const mavenFailureFixture = `
[ERROR] com.example.UserServiceTest.createsUser -- Time elapsed: 0.015 s <<< FAILURE!
java.lang.AssertionError: expected user
    at com.example.UserServiceTest.createsUser(UserServiceTest.java:42)
[ERROR] Tests run: 4, Failures: 1, Errors: 0, Skipped: 0
[ERROR] BUILD FAILURE
`

const javaCompileFixture = `
/workspace/src/main/java/com/example/App.java:12: error: cannot find symbol
        missing();
        ^
  symbol:   method missing()
1 error
`

func TestParseJavaGradleAndMavenFailures(t *testing.T) {
	gradle := parseJavaTestOutput(gradleFailureFixture, "", true)
	if gradle.Confidence != verification.ParseComplete || len(gradle.Failures) != 1 {
		t.Fatalf("gradle=%#v", gradle)
	}
	if failure := gradle.Failures[0]; failure.ID != "com.example.CalculatorTest#addsNumbers" || failure.Class != verification.FailureClassTest {
		t.Fatalf("failure=%#v", failure)
	}

	maven := parseJavaTestOutput(mavenFailureFixture, "", true)
	if maven.Confidence != verification.ParseComplete || len(maven.Failures) != 1 {
		t.Fatalf("maven=%#v", maven)
	}
	if failure := maven.Failures[0]; failure.ID != "com.example.UserServiceTest#createsUser" || failure.Class != verification.FailureClassTest {
		t.Fatalf("failure=%#v", failure)
	}
}

func TestParseJavaCompileFailureIsBuildAndWorkspaceIndependent(t *testing.T) {
	first := parseJavaTestOutput("", javaCompileFixture, true, "/workspace")
	movedText := strings.ReplaceAll(javaCompileFixture, "/workspace", "/other/stage")
	second := parseJavaTestOutput("", movedText, true, "/other/stage")
	if first.Confidence != verification.ParseComplete || len(first.Failures) != 1 {
		t.Fatalf("first=%#v", first)
	}
	if first.Failures[0].Class != verification.FailureClassBuild {
		t.Fatalf("failure=%#v", first.Failures[0])
	}
	if first.Failures[0].Signature != second.Failures[0].Signature {
		t.Fatalf("workspace path changed signature: %s != %s", first.Failures[0].Signature, second.Failures[0].Signature)
	}
}

func TestParseJavaSummaryDetectsPartialCoverage(t *testing.T) {
	text := strings.Replace(gradleFailureFixture, "3 tests completed, 1 failed", "3 tests completed, 2 failed", 1)
	got := parseJavaTestOutput(text, "", true)
	if got.Confidence != verification.ParsePartial || len(got.Failures) != 1 {
		t.Fatalf("got=%#v", got)
	}
}

func TestParseJavaFailedExitWithoutRecognizedFailuresIsFailed(t *testing.T) {
	got := parseJavaTestOutput("FAILURE: Build failed with an exception.", "", true)
	if got.Confidence != verification.ParseFailed || len(got.Failures) != 0 {
		t.Fatalf("got=%#v", got)
	}
}
