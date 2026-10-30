package quality

import (
	"reflect"
	"testing"

	"github.com/FernandoVillegas27/ai-engineering-os/core/verification"
)

func failure(id string, class verification.FailureClass, signature string) verification.Failure {
	return verification.Failure{ID: id, Class: class, Signature: signature}
}

func failedCheck(name string, parse verification.ParseConfidence, failures ...verification.Failure) verification.Check {
	return verification.Check{
		Name: name, Passed: false, FailureKind: verification.FailureCode, Correctable: true,
		Failures: failures, FailureParse: parse,
	}
}

func report(checks ...verification.Check) verification.Report {
	status := verification.StatusPassed
	for _, check := range checks {
		if !check.Passed {
			status = verification.StatusFailed
		}
	}
	return verification.Report{Status: status, Checks: checks}
}

func ids(refs []FailureRef) []string {
	out := []string{}
	for _, ref := range refs {
		out = append(out, ref.Check+":"+ref.Failure.ID)
	}
	return out
}

func TestCompareClassifiesPreexistingRegressionsAndResolved(t *testing.T) {
	baseline := report(failedCheck("go-test", verification.ParseComplete,
		failure("p#TestOld", verification.FailureClassTest, "s1"),
		failure("p#TestFixed", verification.FailureClassTest, "s2"),
	))
	after := report(failedCheck("go-test", verification.ParseComplete,
		failure("p#TestOld", verification.FailureClassTest, "s1"),
		failure("p#TestNew", verification.FailureClassTest, "s3"),
	))

	got := Compare(baseline, after)

	if want := []string{"go-test:p#TestOld"}; !reflect.DeepEqual(ids(got.Preexisting), want) {
		t.Fatalf("preexisting=%v", ids(got.Preexisting))
	}
	if want := []string{"go-test:p#TestNew"}; !reflect.DeepEqual(ids(got.Regressions), want) {
		t.Fatalf("regressions=%v", ids(got.Regressions))
	}
	if want := []string{"go-test:p#TestFixed"}; !reflect.DeepEqual(ids(got.Resolved), want) {
		t.Fatalf("resolved=%v", ids(got.Resolved))
	}
	if len(got.ChangedCause) != 0 || len(got.Unclassified) != 0 {
		t.Fatalf("got=%#v", got)
	}
}

func TestCompareReportsChangedSignatureOrClassAsChangedCause(t *testing.T) {
	baseline := report(failedCheck("go-test", verification.ParseComplete,
		failure("p#TestA", verification.FailureClassTest, "old"),
		failure("p#TestB", verification.FailureClassTest, "same"),
	))
	after := report(failedCheck("go-test", verification.ParseComplete,
		failure("p#TestA", verification.FailureClassTest, "new"),
		failure("p#TestB", verification.FailureClassPanic, "same"),
	))

	got := Compare(baseline, after)

	if len(got.ChangedCause) != 2 || len(got.Preexisting) != 0 || len(got.Regressions) != 0 {
		t.Fatalf("got=%#v", got)
	}
	if got.ChangedCause[0].Before.Signature != "old" || got.ChangedCause[0].After.Signature != "new" {
		t.Fatalf("changed=%#v", got.ChangedCause[0])
	}
}

func TestCompareTreatsEverythingAsRegressionWhenBaselineCheckPassed(t *testing.T) {
	baseline := report(verification.Check{Name: "go-test", Passed: true})
	after := report(failedCheck("go-test", verification.ParseComplete, failure("p#T", verification.FailureClassTest, "s")))

	got := Compare(baseline, after)
	if want := []string{"go-test:p#T"}; !reflect.DeepEqual(ids(got.Regressions), want) {
		t.Fatalf("got=%#v", got)
	}
}

func TestCompareTreatsNewCheckWithoutBaselineAsRegression(t *testing.T) {
	after := report(failedCheck("go-test", verification.ParseComplete, failure("p#T", verification.FailureClassTest, "s")))
	got := Compare(verification.Report{Status: verification.StatusSkipped}, after)
	if len(got.Regressions) != 1 || len(got.Preexisting) != 0 {
		t.Fatalf("got=%#v", got)
	}
}

func TestCompareResolvesFailuresWhenCheckNowPasses(t *testing.T) {
	baseline := report(failedCheck("go-test", verification.ParseComplete, failure("p#T", verification.FailureClassTest, "s")))
	after := report(verification.Check{Name: "go-test", Passed: true})

	got := Compare(baseline, after)
	if want := []string{"go-test:p#T"}; !reflect.DeepEqual(ids(got.Resolved), want) || len(got.Unclassified) != 0 {
		t.Fatalf("got=%#v", got)
	}
}

func TestCompareNeverClassifiesWhatItCannotProve(t *testing.T) {
	complete := failedCheck("go-test", verification.ParseComplete, failure("p#T", verification.FailureClassTest, "s"))
	cases := map[string]struct {
		baseline verification.Report
		after    verification.Report
	}{
		"after not attempted (non-Go ecosystem)": {report(complete), report(failedCheck("go-test", verification.ParseNotAttempted))},
		"after partial":                          {report(complete), report(failedCheck("go-test", verification.ParsePartial, failure("p#T", verification.FailureClassTest, "s")))},
		"after parse failed":                     {report(complete), report(failedCheck("go-test", verification.ParseFailed))},
		"baseline partial":                       {report(failedCheck("go-test", verification.ParsePartial, failure("p#T", verification.FailureClassTest, "s"))), report(complete)},
		"baseline not attempted":                 {report(failedCheck("go-test", verification.ParseNotAttempted)), report(complete)},
		"complete but no failures listed":        {report(complete), report(failedCheck("go-test", verification.ParseComplete))},
		"duplicate IDs after": {report(complete), report(failedCheck("go-test", verification.ParseComplete,
			failure("p#T", verification.FailureClassTest, "s"), failure("p#T", verification.FailureClassTest, "s")))},
		"infrastructure after": {report(complete), report(verification.Check{
			Name: "go-test", FailureKind: verification.FailureInfrastructure, FailureParse: verification.ParseComplete,
			Failures: []verification.Failure{failure("p#T", verification.FailureClassTest, "s")}})},
		"infrastructure in baseline": {report(verification.Check{
			Name: "go-test", FailureKind: verification.FailureInfrastructure, FailureParse: verification.ParseComplete,
			Failures: []verification.Failure{failure("p#T", verification.FailureClassTest, "s")}}), report(complete)},
		"check absent after": {report(complete), report()},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := Compare(tc.baseline, tc.after)
			if len(got.Unclassified) != 1 {
				t.Fatalf("expected exactly one unclassified entry, got %#v", got)
			}
			if len(got.Preexisting) != 0 || len(got.Regressions) != 0 || len(got.Resolved) != 0 || len(got.ChangedCause) != 0 {
				t.Fatalf("unproven checks must not be classified: %#v", got)
			}
			if got.Unclassified[0].Check != "go-test" || got.Unclassified[0].Reason == "" {
				t.Fatalf("unclassified=%#v", got.Unclassified[0])
			}
		})
	}
}

func TestCompareKeepsChecksIndependentAndDeterministic(t *testing.T) {
	baseline := report(
		failedCheck("pytest", verification.ParseNotAttempted),
		failedCheck("go-test", verification.ParseComplete, failure("p#A", verification.FailureClassTest, "s")),
	)
	after := report(
		failedCheck("go-test", verification.ParseComplete,
			failure("p#B", verification.FailureClassTest, "x"),
			failure("p#A", verification.FailureClassTest, "s")),
		failedCheck("pytest", verification.ParseNotAttempted),
	)

	first := Compare(baseline, after)
	second := Compare(baseline, after)
	if !reflect.DeepEqual(first, second) {
		t.Fatal("comparison must be deterministic")
	}
	if want := []string{"go-test:p#A"}; !reflect.DeepEqual(ids(first.Preexisting), want) {
		t.Fatalf("preexisting=%v", ids(first.Preexisting))
	}
	if want := []string{"go-test:p#B"}; !reflect.DeepEqual(ids(first.Regressions), want) {
		t.Fatalf("regressions=%v", ids(first.Regressions))
	}
	if len(first.Unclassified) != 1 || first.Unclassified[0].Check != "pytest" {
		t.Fatalf("unclassified=%#v", first.Unclassified)
	}
}
