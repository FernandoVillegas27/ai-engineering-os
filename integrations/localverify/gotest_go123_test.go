package localverify

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/FernandoVillegas27/ai-engineering-os/core/verification"
)

// Captured with go1.23.12: compile errors are printed to stderr, outside the JSON stream.
const fixtureGo123BuildStdout = `{"Action":"start","Package":"probe/app"}
{"Action":"output","Package":"probe/app","Output":"FAIL\tprobe/app [build failed]\n"}
{"Action":"fail","Package":"probe/app","Elapsed":0}
{"Action":"start","Package":"probe/lib"}
{"Action":"output","Package":"probe/lib","Output":"FAIL\tprobe/lib [build failed]\n"}
{"Action":"fail","Package":"probe/lib","Elapsed":0.001}
{"Action":"start","Package":"probe/oops"}
{"Action":"output","Package":"probe/oops","Output":"FAIL\tprobe/oops [build failed]\n"}
{"Action":"fail","Package":"probe/oops","Elapsed":0.002}
{"Action":"start","Package":"probe/other"}
{"Action":"output","Package":"probe/other","Output":"FAIL\tprobe/other [build failed]\n"}
{"Action":"fail","Package":"probe/other","Elapsed":0}
`

const fixtureGo123BuildStderr = `# probe/lib
lib\lib.go:2:23: undefined: missing
# probe/oops [probe/oops.test]
oops\o_test.go:3:27: undefined: undefinedOne
# probe/other [probe/other.test]
other\p_test.go:3:27: undefined: undefinedTwo
`

func TestGo123BuildFailuresAreClassifiedAsBuildWithDistinctSignatures(t *testing.T) {
	got := parseGoTestJSONWithStderr(fixtureGo123BuildStdout, fixtureGo123BuildStderr, true)

	if got.Confidence != verification.ParseComplete || len(got.Failures) != 4 {
		t.Fatalf("got=%#v", got)
	}
	signatures := map[string]string{}
	for _, failure := range got.Failures {
		if failure.Class != verification.FailureClassBuild {
			t.Fatalf("%s must be a build failure, got %s", failure.ID, failure.Class)
		}
		for other, signature := range signatures {
			if signature == failure.Signature {
				t.Fatalf("%s and %s share a signature although their compiler errors differ", other, failure.ID)
			}
		}
		signatures[failure.ID] = failure.Signature
	}
}

func TestGo123BuildSignatureFollowsItsOwnCompilerError(t *testing.T) {
	baseline := parseGoTestJSONWithStderr(fixtureGo123BuildStdout, fixtureGo123BuildStderr, true)

	sameAgain := parseGoTestJSONWithStderr(fixtureGo123BuildStdout, fixtureGo123BuildStderr, true)
	for i := range baseline.Failures {
		if baseline.Failures[i] != sameAgain.Failures[i] {
			t.Fatalf("identical output must give identical failures: %#v vs %#v", baseline.Failures[i], sameAgain.Failures[i])
		}
	}

	changed := strings.ReplaceAll(fixtureGo123BuildStderr, "undefined: undefinedOne", "undefined: somethingElse")
	after := parseGoTestJSONWithStderr(fixtureGo123BuildStdout, changed, true)
	if failureByID(t, after.Failures, "probe/oops").Signature == failureByID(t, baseline.Failures, "probe/oops").Signature {
		t.Fatal("a different compiler error must change that package's signature")
	}
	if failureByID(t, after.Failures, "probe/other").Signature != failureByID(t, baseline.Failures, "probe/other").Signature {
		t.Fatal("an unrelated package must keep its signature")
	}
}

func TestGo123DependentBuildFailureUsesAllCompilerOutput(t *testing.T) {
	baseline := parseGoTestJSONWithStderr(fixtureGo123BuildStdout, fixtureGo123BuildStderr, true)
	changed := strings.ReplaceAll(fixtureGo123BuildStderr, "undefined: missing", "undefined: gone")
	after := parseGoTestJSONWithStderr(fixtureGo123BuildStdout, changed, true)

	// probe/app has no compiler section of its own, so any change in any error must show.
	if failureByID(t, after.Failures, "probe/app").Signature == failureByID(t, baseline.Failures, "probe/app").Signature {
		t.Fatal("a dependent package must not hide a changed build error")
	}
}

func TestGo123BuildFailureWithoutStderrIsOnlyPartiallyUnderstood(t *testing.T) {
	got := parseGoTestJSONWithStderr(fixtureGo123BuildStdout, "", true)
	if got.Confidence != verification.ParsePartial {
		t.Fatalf("a signature built without compiler text must not be trusted, confidence=%q", got.Confidence)
	}
}

func TestGo123BuildSignaturesIgnoreWorkspaceLocation(t *testing.T) {
	stderr := "# probe/oops [probe/oops.test]\nC:\\ws\\a\\oops\\o_test.go:3:27: undefined: undefinedOne\n"
	moved := strings.ReplaceAll(stderr, `C:\ws\a`, `D:\other\b`)
	stdout := `{"Action":"start","Package":"probe/oops"}
{"Action":"output","Package":"probe/oops","Output":"FAIL\tprobe/oops [build failed]\n"}
{"Action":"fail","Package":"probe/oops"}
`
	first := parseGoTestJSONWithStderr(stdout, stderr, true, `C:\ws\a`)
	second := parseGoTestJSONWithStderr(stdout, moved, true, `D:\other\b`)
	if failureByID(t, first.Failures, "probe/oops").Signature != failureByID(t, second.Failures, "probe/oops").Signature {
		t.Fatal("workspace paths in compiler output must not change the signature")
	}
}

func TestVerifyPassesStderrToTheParserForLegacyBuildErrors(t *testing.T) {
	root := t.TempDir()
	writeGoMod(t, root)
	report, err := (Runtime{Runner: streamRunner{stdout: fixtureGo123BuildStdout, stderr: fixtureGo123BuildStderr, err: os.ErrInvalid}}).Verify(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	check := report.Checks[0]
	if check.FailureParse != verification.ParseComplete || len(check.Failures) != 4 {
		t.Fatalf("check=%#v", check)
	}
	if check.FailureKind != verification.FailureCode {
		t.Fatalf("a compile error is a code failure, not infrastructure: %#v", check)
	}
}
