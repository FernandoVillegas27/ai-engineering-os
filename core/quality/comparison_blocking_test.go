package quality

import (
	"reflect"
	"testing"

	"github.com/FernandoVillegas27/ai-engineering-os/core/verification"
)

func TestCompareKeepsCompileErrorsBlockingWhateverTheirClassification(t *testing.T) {
	build := failure("p", verification.FailureClassBuild, "same")
	test := failure("p#T", verification.FailureClassTest, "s")

	cases := map[string]struct {
		baseline verification.Report
		after    verification.Report
		wantIn   string
	}{
		"preexisting build error": {
			report(failedCheck("go-test", verification.ParseComplete, build)),
			report(failedCheck("go-test", verification.ParseComplete, build, test)),
			"preexisting",
		},
		"new build error": {
			report(verification.Check{Name: "go-test", Passed: true}),
			report(failedCheck("go-test", verification.ParseComplete, build)),
			"regression",
		},
		"changed build error": {
			report(failedCheck("go-test", verification.ParseComplete, failure("p", verification.FailureClassBuild, "old"))),
			report(failedCheck("go-test", verification.ParseComplete, failure("p", verification.FailureClassBuild, "new"))),
			"changed",
		},
		"build error in an unclassifiable check": {
			report(failedCheck("go-test", verification.ParseNotAttempted)),
			report(failedCheck("go-test", verification.ParsePartial, build)),
			"unclassified",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := Compare(tc.baseline, tc.after)
			if len(got.Blocking) != 1 || got.Blocking[0].Failure.ID != "p" || got.Blocking[0].Check != "go-test" {
				t.Fatalf("blocking=%#v", got.Blocking)
			}
			switch tc.wantIn {
			case "preexisting":
				if len(got.Preexisting) == 0 {
					t.Fatalf("test setup: expected the build error to be preexisting: %#v", got)
				}
			case "regression":
				if len(got.Regressions) == 0 {
					t.Fatalf("got=%#v", got)
				}
			case "changed":
				if len(got.ChangedCause) == 0 {
					t.Fatalf("got=%#v", got)
				}
			case "unclassified":
				if len(got.Unclassified) == 0 {
					t.Fatalf("got=%#v", got)
				}
			}
		})
	}
}

func TestCompareBlockingIgnoresNonBuildFailuresAndPassingChecks(t *testing.T) {
	baseline := report(failedCheck("go-test", verification.ParseComplete, failure("p#T", verification.FailureClassTest, "s")))
	got := Compare(baseline, baseline)
	if len(got.Blocking) != 0 || len(got.Preexisting) != 1 {
		t.Fatalf("got=%#v", got)
	}

	resolved := Compare(report(failedCheck("go-test", verification.ParseComplete, failure("p", verification.FailureClassBuild, "s"))),
		report(verification.Check{Name: "go-test", Passed: true}))
	if len(resolved.Blocking) != 0 || len(resolved.Resolved) != 1 {
		t.Fatalf("a fixed compile error must not remain blocking: %#v", resolved)
	}
}

func TestCompareBlockingIsDeterministic(t *testing.T) {
	after := report(failedCheck("go-test", verification.ParseComplete,
		failure("b", verification.FailureClassBuild, "1"), failure("a", verification.FailureClassBuild, "2")))
	got := Compare(after, after)
	want := []string{"go-test:a", "go-test:b"}
	if !reflect.DeepEqual(ids(got.Blocking), want) {
		t.Fatalf("blocking=%v want %v", ids(got.Blocking), want)
	}
}
