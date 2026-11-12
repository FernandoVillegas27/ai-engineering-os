package localverify

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FernandoVillegas27/ai-engineering-os/core/verification"
	"github.com/FernandoVillegas27/ai-engineering-os/integrations/commandmodel"
)

func failureByID(t *testing.T, failures []verification.Failure, id string) verification.Failure {
	t.Helper()
	for _, failure := range failures {
		if failure.ID == id {
			return failure
		}
	}
	t.Fatalf("failure %q not found in %#v", id, failures)
	return verification.Failure{}
}

func TestParseGoTestJSONClassifiesTestsSubtestsPanicsAndBuildErrors(t *testing.T) {
	got := parseGoTestJSON(fixtureMixedFailures, true, "C:/work/ws-a")

	if got.Confidence != verification.ParseComplete {
		t.Fatalf("confidence=%q", got.Confidence)
	}
	want := map[string]verification.FailureClass{
		"probe/bad#TestOther":   verification.FailureClassTest,
		"probe/boom#TestBoom":   verification.FailureClassPanic,
		"probe/broken":          verification.FailureClassBuild,
		"probe/ok#TestFail":     verification.FailureClassTest,
		"probe/ok#TestFail/sub": verification.FailureClassSubtest,
	}
	if len(got.Failures) != len(want) {
		t.Fatalf("failures=%#v", got.Failures)
	}
	for id, class := range want {
		if failure := failureByID(t, got.Failures, id); failure.Class != class || len(failure.Signature) != 64 {
			t.Fatalf("%s=%#v want class %s", id, failure, class)
		}
	}
	for i := 1; i < len(got.Failures); i++ {
		if got.Failures[i-1].ID >= got.Failures[i].ID {
			t.Fatalf("failures must be sorted by ID: %#v", got.Failures)
		}
	}
}

func TestParseGoTestJSONRendersPlainTextForPresentation(t *testing.T) {
	got := parseGoTestJSON(fixtureMixedFailures, true)
	for _, expected := range []string{"--- FAIL: TestOther", "undefined: undefinedSymbol", "FAIL\tprobe/bad\t0.152s", "panic: kaboom"} {
		if !strings.Contains(got.Rendered, expected) {
			t.Fatalf("rendered output missing %q:\n%s", expected, got.Rendered)
		}
	}
	if strings.Contains(got.Rendered, `"Action"`) {
		t.Fatal("rendered output must not contain raw JSON events")
	}
}

func TestParseGoTestJSONPackageSummaryIsNotReportedSeparately(t *testing.T) {
	got := parseGoTestJSON(fixtureMixedFailures, true)
	for _, failure := range got.Failures {
		if failure.ID == "probe/bad" || failure.ID == "probe/ok" || failure.ID == "probe/boom" {
			t.Fatalf("derived package failure reported: %#v", failure)
		}
	}
}

func TestParseGoTestJSONReportsPackageFailureOutsideTests(t *testing.T) {
	got := parseGoTestJSON(fixturePackageFailureWithoutTests, true)
	if got.Confidence != verification.ParseComplete || len(got.Failures) != 1 {
		t.Fatalf("got=%#v", got)
	}
	if failure := got.Failures[0]; failure.ID != "probe/main" || failure.Class != verification.FailureClassPackage {
		t.Fatalf("failure=%#v", failure)
	}
}

func TestParseGoTestJSONDependentBuildFailureUsesLibraryCompilerOutput(t *testing.T) {
	got := parseGoTestJSON(fixtureDependentBuildFailure, true)
	if len(got.Failures) != 1 {
		t.Fatalf("failures=%#v", got.Failures)
	}
	app := failureByID(t, got.Failures, "probe/app")
	if app.Class != verification.FailureClassBuild {
		t.Fatalf("app=%#v", app)
	}
	want := verification.Signature("# probe/lib\nlib\\lib.go:4:2: undefined: missing\n")
	if app.Signature != want {
		t.Fatalf("signature must come from the library compiler output: %s != %s", app.Signature, want)
	}
}

func TestParseGoTestJSONBuildFailureWithoutPackageEventIsReportedOnItsOwn(t *testing.T) {
	stream := `{"ImportPath":"probe/lib","Action":"build-output","Output":"lib.go:4:2: undefined: missing\n"}
{"ImportPath":"probe/lib","Action":"build-fail"}
`
	got := parseGoTestJSON(stream, true)
	if got.Confidence != verification.ParseComplete || len(got.Failures) != 1 {
		t.Fatalf("got=%#v", got)
	}
	if failure := got.Failures[0]; failure.ID != "probe/lib" || failure.Class != verification.FailureClassBuild {
		t.Fatalf("failure=%#v", failure)
	}
}

func TestParseGoTestJSONPassingStreamHasNoFailures(t *testing.T) {
	got := parseGoTestJSON(fixtureAllPass, false)
	if len(got.Failures) != 0 || got.Confidence != verification.ParseComplete {
		t.Fatalf("got=%#v", got)
	}
}

func TestParseGoTestJSONTruncatedStreamIsPartial(t *testing.T) {
	got := parseGoTestJSON(fixtureTruncatedStream, true)
	if got.Confidence != verification.ParsePartial {
		t.Fatalf("confidence=%q", got.Confidence)
	}
}

func TestParseGoTestJSONGarbageLinesMakeParsePartialAndAreKept(t *testing.T) {
	got := parseGoTestJSON(fixtureWithGarbageLines, true)
	if got.Confidence != verification.ParsePartial {
		t.Fatalf("confidence=%q", got.Confidence)
	}
	if !strings.Contains(got.Rendered, "warning: something printed outside the event stream") {
		t.Fatalf("unparsed line dropped: %q", got.Rendered)
	}
	failureByID(t, got.Failures, "probe/bad#TestOther")
}

func TestParseGoTestJSONFailedExitWithoutEventsIsNotUnderstood(t *testing.T) {
	got := parseGoTestJSON("go: cannot find main module", true)
	if got.Confidence != verification.ParseFailed || got.Rendered != "go: cannot find main module" {
		t.Fatalf("got=%#v", got)
	}
	if empty := parseGoTestJSON("", false); empty.Confidence != verification.ParseNotAttempted {
		t.Fatalf("empty=%#v", empty)
	}
}

func TestParseGoTestJSONFailedExitWithOnlyPassingEventsIsNotUnderstood(t *testing.T) {
	got := parseGoTestJSON(fixtureAllPass, true)
	if got.Confidence != verification.ParseFailed || len(got.Failures) != 0 {
		t.Fatalf("got=%#v", got)
	}
}

func TestParseGoTestJSONSignaturesIgnoreWorkspaceLocationButNotRealChanges(t *testing.T) {
	baseline := parseGoTestJSON(fixtureMixedFailures, true, "C:/work/ws-a")
	moved := strings.ReplaceAll(fixtureMixedFailures, "C:/work/ws-a", "D:/other/ws-b")
	staged := parseGoTestJSON(moved, true, "D:/other/ws-b")

	if got, want := failureByID(t, staged.Failures, "probe/boom#TestBoom").Signature, failureByID(t, baseline.Failures, "probe/boom#TestBoom").Signature; got != want {
		t.Fatalf("workspace path changed the signature: %s != %s", got, want)
	}

	shifted := strings.ReplaceAll(fixtureMixedFailures, "b_test.go:3: x", "b_test.go:9: x")
	changedLine := parseGoTestJSON(shifted, true, "C:/work/ws-a")
	if failureByID(t, changedLine.Failures, "probe/bad#TestOther").Signature == failureByID(t, baseline.Failures, "probe/bad#TestOther").Signature {
		t.Fatal("a different line number must change the signature")
	}
	reworded := strings.ReplaceAll(fixtureMixedFailures, "undefined: undefinedSymbol", "undefined: otherSymbol")
	if failureByID(t, parseGoTestJSON(reworded, true).Failures, "probe/broken").Signature == failureByID(t, baseline.Failures, "probe/broken").Signature {
		t.Fatal("a different compiler message must change the signature")
	}
}

type streamRunner struct {
	stdout string
	stderr string
	err    error
}

func (streamRunner) LookPath(file string) (string, error) { return file, nil }

func (s streamRunner) Run(context.Context, commandmodel.Invocation) (string, string, error) {
	return s.stdout, s.stderr, s.err
}

func writeGoMod(t *testing.T, root string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/test\n\ngo 1.23\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyRunsGoTestWithJSONAndAttachesFailures(t *testing.T) {
	root := t.TempDir()
	writeGoMod(t, root)
	report, err := (Runtime{Runner: streamRunner{stdout: fixtureMixedFailures, err: os.ErrInvalid}}).Verify(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	check := report.Checks[0]
	if report.Status != verification.StatusFailed || check.Passed {
		t.Fatalf("report=%#v", report)
	}
	if check.FailureKind != verification.FailureCode || !check.Correctable {
		t.Fatalf("check-level classification must be preserved: %#v", check)
	}
	if check.FailureParse != verification.ParseComplete || len(check.Failures) != 5 {
		t.Fatalf("check=%#v", check)
	}
	if strings.Contains(check.Stdout, `"Action"`) || !strings.Contains(check.Stdout, "--- FAIL: TestOther") {
		t.Fatalf("stdout must be readable text: %q", check.Stdout)
	}
}

func TestVerifyParsesCompleteOutputBeforeTruncatingForPresentation(t *testing.T) {
	root := t.TempDir()
	writeGoMod(t, root)

	var stream strings.Builder
	stream.WriteString(`{"Action":"start","Package":"probe/big"}` + "\n")
	filler := strings.Repeat("x", 200)
	for i := 0; i < 100; i++ {
		stream.WriteString(`{"Action":"output","Package":"probe/big","Output":"` + filler + `\n"}` + "\n")
	}
	stream.WriteString(`{"Action":"run","Package":"probe/big","Test":"TestLate"}` + "\n")
	stream.WriteString(`{"Action":"output","Package":"probe/big","Test":"TestLate","Output":"    late_test.go:1: boom\n"}` + "\n")
	stream.WriteString(`{"Action":"fail","Package":"probe/big","Test":"TestLate"}` + "\n")
	stream.WriteString(`{"Action":"fail","Package":"probe/big"}` + "\n")

	report, err := (Runtime{Runner: streamRunner{stdout: stream.String(), err: os.ErrInvalid}}).Verify(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	check := report.Checks[0]
	if !strings.Contains(check.Stdout, "[truncated]") || strings.Contains(check.Stdout, "late_test.go") {
		t.Fatalf("presentation output should be truncated before the late failure (len=%d)", len(check.Stdout))
	}
	failureByID(t, check.Failures, "probe/big#TestLate")
}

func TestVerifyKeepsInfrastructureClassificationWithJSONOutput(t *testing.T) {
	root := t.TempDir()
	writeGoMod(t, root)
	report, err := (Runtime{Runner: streamRunner{stderr: "proxyconnect tcp: 407 Proxy Authentication Required", err: os.ErrInvalid}}).Verify(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	check := report.Checks[0]
	if check.FailureKind != verification.FailureInfrastructure || check.Correctable {
		t.Fatalf("check=%#v", check)
	}
	if check.FailureParse != verification.ParseFailed {
		t.Fatalf("confidence=%q", check.FailureParse)
	}
}

func TestCrossLanguageChecksKeepCommandsAndSafeParseConfidence(t *testing.T) {
	for _, tc := range []struct {
		name       string
		file       string
		content    string
		command    string
		parse      verification.ParseConfidence
		assertArgs func(*testing.T, string, []string)
	}{
		{
			name: "maven", file: "pom.xml", content: "<project/>", command: "mvn",
			parse: verification.ParseFailed,
			assertArgs: func(t *testing.T, _ string, args []string) {
				if strings.Join(args, " ") != "-q test" { t.Fatalf("args=%#v", args) }
			},
		},
		{
			name: "gradle", file: "build.gradle", content: "", command: "gradle",
			parse: verification.ParseFailed,
			assertArgs: func(t *testing.T, _ string, args []string) {
				if strings.Join(args, " ") != "test --no-daemon --console=plain" { t.Fatalf("args=%#v", args) }
			},
		},
		{
			name: "pytest", file: "pyproject.toml", content: "", command: "python",
			parse: verification.ParseFailed,
			assertArgs: func(t *testing.T, root string, args []string) {
				got := strings.Join(args, " ")
				if !strings.HasPrefix(got, "-m pytest -r fE --junitxml=") {
					t.Fatalf("args=%#v", args)
				}
				assertReportArgOutsideWorkspace(t, root, args[len(args)-1], "--junitxml=")
			},
		},
		{
			name: "jest", file: "package.json", content: `{"scripts":{"test":"jest"}}`, command: "npm",
			parse: verification.ParseFailed,
			assertArgs: func(t *testing.T, root string, args []string) {
				got := strings.Join(args, " ")
				if !strings.HasPrefix(got, "test -- --runInBand --json --outputFile=") {
					t.Fatalf("args=%#v", args)
				}
				assertReportArgOutsideWorkspace(t, root, args[len(args)-1], "--outputFile=")
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, tc.file), []byte(tc.content), 0o600); err != nil {
				t.Fatal(err)
			}
			runner := &fakeRunner{fail: true, stderr: "FAIL something"}
			report, err := (Runtime{Runner: runner}).Verify(context.Background(), root)
			if err != nil {
				t.Fatal(err)
			}
			invocation := runner.invocations[0]
			if filepath.Base(invocation.Command) != tc.command {
				t.Fatalf("invocation=%#v", invocation)
			}
			tc.assertArgs(t, root, invocation.Args)
			check := report.Checks[0]
			if check.FailureParse != tc.parse || check.Failures != nil {
				t.Fatalf("check=%#v", check)
			}
			if check.FailureKind != verification.FailureCode || !check.Correctable {
				t.Fatalf("check-level classification must be unchanged: %#v", check)
			}
		})
	}
}

func assertReportArgOutsideWorkspace(t *testing.T, root, arg, prefix string) {
	t.Helper()
	path := strings.TrimPrefix(arg, prefix)
	if path == arg || strings.TrimSpace(path) == "" {
		t.Fatalf("report arg %q missing prefix %q", arg, prefix)
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		t.Fatal(err)
	}
	if rel == "." || !strings.HasPrefix(filepath.ToSlash(rel), "../") {
		t.Fatalf("report path should be outside workspace: root=%q path=%q", root, path)
	}
}

// goCacheRunner reuses the developer's build cache so the real-process test stays fast.
type goCacheRunner struct{ cache string }

func (r goCacheRunner) LookPath(file string) (string, error) { return exec.LookPath(file) }

func (r goCacheRunner) Run(ctx context.Context, invocation commandmodel.Invocation) (string, string, error) {
	env := make([]string, 0, len(invocation.Env))
	for _, entry := range invocation.Env {
		if !strings.HasPrefix(entry, "GOCACHE=") {
			env = append(env, entry)
		}
	}
	invocation.Env = append(env, "GOCACHE="+r.cache)
	return commandmodel.OSRunner{}.Run(ctx, invocation)
}

func TestVerifyParsesRealGoTestJSONOutput(t *testing.T) {
	if testing.Short() {
		t.Skip("runs a real go test process")
	}
	goBinary, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go toolchain not available")
	}
	cacheOutput, err := exec.Command(goBinary, "env", "GOCACHE").Output()
	if err != nil {
		t.Skipf("cannot resolve GOCACHE: %v", err)
	}

	root := t.TempDir()
	files := map[string]string{
		"go.mod":            "module example.com/real\n\ngo 1.23\n",
		"ok/ok_test.go":     "package ok\n\nimport \"testing\"\n\nfunc TestPass(t *testing.T) {}\n",
		"bad/bad_test.go":   "package bad\n\nimport \"testing\"\n\nfunc TestFail(t *testing.T) {\n\tt.Run(\"sub\", func(t *testing.T) { t.Fatal(\"nope\") })\n}\n",
		"oops/oops_test.go": "package oops\n\nimport \"testing\"\n\nfunc TestOops(t *testing.T) { undefinedSymbol() }\n",
	}
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	runtime := Runtime{Runner: goCacheRunner{cache: strings.TrimSpace(string(cacheOutput))}}
	first, err := runtime.Verify(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	check := first.Checks[0]
	if check.Passed || check.FailureParse != verification.ParseComplete {
		t.Fatalf("check=%#v", check)
	}
	failureByID(t, check.Failures, "example.com/real/bad#TestFail")
	failureByID(t, check.Failures, "example.com/real/bad#TestFail/sub")
	if oops := failureByID(t, check.Failures, "example.com/real/oops"); oops.Class != verification.FailureClassBuild {
		t.Fatalf("compile failure=%#v", oops)
	}
	for _, failure := range check.Failures {
		if failure.ID == "example.com/real/ok#TestPass" {
			t.Fatal("passing test reported as failure")
		}
	}

	// A second run in a different workspace location must produce identical signatures.
	moved := t.TempDir()
	for name, content := range files {
		path := filepath.Join(moved, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	second, err := runtime.Verify(context.Background(), moved)
	if err != nil {
		t.Fatal(err)
	}
	for _, failure := range check.Failures {
		other := failureByID(t, second.Checks[0].Failures, failure.ID)
		if other.Signature != failure.Signature || other.Class != failure.Class {
			t.Fatalf("%s differs between workspaces:\n%#v\n%#v", failure.ID, failure, other)
		}
	}
}

func TestParseGoTestJSONUnknownActionIsPartial(t *testing.T) {
	raw := "{\"Action\":\"start\",\"Package\":\"example/pkg\"}\n" +
		"{\"Action\":\"future-action\",\"Package\":\"example/pkg\"}\n" +
		"{\"Action\":\"pass\",\"Package\":\"example/pkg\"}\n"
	got := parseGoTestJSON(raw, false)
	if got.Confidence != verification.ParsePartial {
		t.Fatalf("unknown action must not produce complete evidence: %q", got.Confidence)
	}
}

func TestParseGoTestJSONKnownActionsRemainComplete(t *testing.T) {
	raw := "{\"Action\":\"start\",\"Package\":\"example/pkg\"}\n" +
		"{\"Action\":\"pass\",\"Package\":\"example/pkg\"}\n"
	got := parseGoTestJSON(raw, false)
	if got.Confidence != verification.ParseComplete {
		t.Fatalf("known successful events should be complete: %q", got.Confidence)
	}
}

func TestParseGoTestJSONUnscopedLifecycleEventIsPartial(t *testing.T) {
	for _, action := range []string{"start", "run", "pass", "fail", "skip"} {
		t.Run(action, func(t *testing.T) {
			raw := "{\"Action\":\"start\",\"Package\":\"example/pkg\"}\n" +
				"{\"Action\":\"" + action + "\"}\n" +
				"{\"Action\":\"pass\",\"Package\":\"example/pkg\"}\n"
			got := parseGoTestJSON(raw, false)
			if got.Confidence != verification.ParsePartial {
				t.Fatalf("unscoped %s must yield partial evidence, got %q", action, got.Confidence)
			}
		})
	}
}

func TestParseGoTestJSONScopedLifecycleEventsRemainComplete(t *testing.T) {
	raw := "{\"Action\":\"start\",\"Package\":\"example/pkg\"}\n" +
		"{\"Action\":\"run\",\"Package\":\"example/pkg\",\"Test\":\"TestOK\"}\n" +
		"{\"Action\":\"pass\",\"Package\":\"example/pkg\",\"Test\":\"TestOK\"}\n" +
		"{\"Action\":\"pass\",\"Package\":\"example/pkg\"}\n"
	got := parseGoTestJSON(raw, false)
	if got.Confidence != verification.ParseComplete {
		t.Fatalf("scoped lifecycle events should remain complete, got %q", got.Confidence)
	}
}

func TestParseGoTestJSONRejectsBuildEventsWithoutImportPath(t *testing.T) {
	for _, action := range []string{"build-output", "build-fail"} {
		for _, importPath := range []string{"", "   ", " [probe.test]"} {
			t.Run(action+"/"+strings.ReplaceAll(importPath, " ", "_"), func(t *testing.T) {
				stream := "{\"Action\":\"start\",\"Package\":\"probe/ok\"}\n" +
					"{\"Action\":\"pass\",\"Package\":\"probe/ok\"}\n" +
					"{\"Action\":\"" + action + "\",\"ImportPath\":\"" + importPath + "\",\"Output\":\"compiler error\\n\"}\n"
				got := parseGoTestJSON(stream, false)
				if got.Confidence != verification.ParsePartial {
					t.Fatalf("unattributable %s must be partial, got %#v", action, got)
				}
				if len(got.Failures) != 0 {
					t.Fatalf("unattributable build event must not create empty failure identity: %#v", got.Failures)
				}
			})
		}
	}
}

func TestParseGoTestJSONAcceptsScopedBuildEvents(t *testing.T) {
	stream := "{\"Action\":\"build-output\",\"ImportPath\":\"probe/lib [probe/lib.test]\",\"Output\":\"compiler error\\n\"}\n" +
		"{\"Action\":\"build-fail\",\"ImportPath\":\"probe/lib [probe/lib.test]\"}\n"
	got := parseGoTestJSON(stream, true)
	if got.Confidence != verification.ParseComplete || len(got.Failures) != 1 || got.Failures[0].ID != "probe/lib" {
		t.Fatalf("scoped build failure must remain complete: %#v", got)
	}
}

func TestParseGoTestJSONRejectsUnscopedOutput(t *testing.T) {
	for _, pkg := range []string{"", "   "} {
		t.Run("package_"+strings.ReplaceAll(pkg, " ", "_"), func(t *testing.T) {
			stream := "{\"Action\":\"start\",\"Package\":\"probe/ok\"}\n" +
				"{\"Action\":\"output\",\"Package\":\"" + pkg + "\",\"Output\":\"unattributed text\\n\"}\n" +
				"{\"Action\":\"pass\",\"Package\":\"probe/ok\"}\n"
			got := parseGoTestJSON(stream, false)
			if got.Confidence != verification.ParsePartial {
				t.Fatalf("unscoped output must be partial, got %#v", got)
			}
			if strings.Contains(got.Rendered, "unattributed text") {
				t.Fatalf("unscoped output must not be rendered as attributed output: %q", got.Rendered)
			}
		})
	}
}

func TestParseGoTestJSONScopedOutputRemainsComplete(t *testing.T) {
	stream := "{\"Action\":\"start\",\"Package\":\"probe/ok\"}\n" +
		"{\"Action\":\"output\",\"Package\":\"probe/ok\",\"Output\":\"ok\\n\"}\n" +
		"{\"Action\":\"pass\",\"Package\":\"probe/ok\"}\n"
	got := parseGoTestJSON(stream, false)
	if got.Confidence != verification.ParseComplete || !strings.Contains(got.Rendered, "ok") {
		t.Fatalf("scoped output must remain complete, got %#v", got)
	}
}

func TestParseGoTestJSONRejectsTestScopedStart(t *testing.T) {
	stream := "{\"Action\":\"start\",\"Package\":\"probe/ok\",\"Test\":\"TestUnexpected\"}\n" +
		"{\"Action\":\"start\",\"Package\":\"probe/ok\"}\n" +
		"{\"Action\":\"pass\",\"Package\":\"probe/ok\"}\n"
	got := parseGoTestJSON(stream, false)
	if got.Confidence != verification.ParsePartial {
		t.Fatalf("test-scoped start must not be complete evidence: %#v", got)
	}
}

func TestParseGoTestJSONPackageStartRemainsComplete(t *testing.T) {
	stream := "{\"Action\":\"start\",\"Package\":\"probe/ok\"}\n" +
		"{\"Action\":\"pass\",\"Package\":\"probe/ok\"}\n"
	got := parseGoTestJSON(stream, false)
	if got.Confidence != verification.ParseComplete {
		t.Fatalf("package-scoped start must remain complete: %#v", got)
	}
}


func TestParseGoTestJSONRejectsUnscopedRun(t *testing.T) {
	for _, testName := range []string{"", "   "} {
		stream := "{\"Action\":\"start\",\"Package\":\"probe/ok\"}\n" +
			"{\"Action\":\"run\",\"Package\":\"probe/ok\",\"Test\":\"" + testName + "\"}\n" +
			"{\"Action\":\"pass\",\"Package\":\"probe/ok\"}\n"
		got := parseGoTestJSON(stream, false)
		if got.Confidence != verification.ParsePartial {
			t.Fatalf("run without a test identity must be partial: %#v", got)
		}
	}
}

func TestParseGoTestJSONScopedRunRemainsComplete(t *testing.T) {
	stream := "{\"Action\":\"start\",\"Package\":\"probe/ok\"}\n" +
		"{\"Action\":\"run\",\"Package\":\"probe/ok\",\"Test\":\"TestOK\"}\n" +
		"{\"Action\":\"pass\",\"Package\":\"probe/ok\",\"Test\":\"TestOK\"}\n" +
		"{\"Action\":\"pass\",\"Package\":\"probe/ok\"}\n"
	got := parseGoTestJSON(stream, false)
	if got.Confidence != verification.ParseComplete {
		t.Fatalf("test-scoped run must remain complete: %#v", got)
	}
}

func TestParseGoTestJSONRejectsWhitespaceOnlyTerminalTestIdentity(t *testing.T) {
	for _, action := range []string{"pass", "fail", "skip"} {
		t.Run(action, func(t *testing.T) {
			stream := "{\"Action\":\"start\",\"Package\":\"probe/ok\"}\n" +
				"{\"Action\":\"" + action + "\",\"Package\":\"probe/ok\",\"Test\":\"   \"}\n" +
				"{\"Action\":\"pass\",\"Package\":\"probe/ok\"}\n"
			got := parseGoTestJSON(stream, false)
			if got.Confidence != verification.ParsePartial || len(got.Failures) != 0 {
				t.Fatalf("invalid terminal event must be partial without failures: %#v", got)
			}
		})
	}
}

func TestParseGoTestJSONValidTerminalEventsRemainComplete(t *testing.T) {
	for _, action := range []string{"pass", "fail", "skip"} {
		t.Run(action, func(t *testing.T) {
			stream := "{\"Action\":\"start\",\"Package\":\"probe/ok\"}\n" +
				"{\"Action\":\"run\",\"Package\":\"probe/ok\",\"Test\":\"TestOK\"}\n" +
				"{\"Action\":\"" + action + "\",\"Package\":\"probe/ok\",\"Test\":\"TestOK\"}\n" +
				"{\"Action\":\"pass\",\"Package\":\"probe/ok\"}\n"
			got := parseGoTestJSON(stream, action == "fail")
			if got.Confidence != verification.ParseComplete {
				t.Fatalf("valid terminal event must remain complete: %#v", got)
			}
		})
	}
}


func TestParseGoTestJSONRejectsWhitespaceOnlyOutputTestIdentity(t *testing.T) {
	stream := "{\"Action\":\"start\",\"Package\":\"probe/ok\"}\\n" +
		"{\"Action\":\"output\",\"Package\":\"probe/ok\",\"Test\":\"   \",\"Output\":\"unattributed test output\\n\"}\\n" +
		"{\"Action\":\"pass\",\"Package\":\"probe/ok\"}\\n"
	got := parseGoTestJSON(stream, false)
	if got.Confidence != verification.ParsePartial || strings.Contains(got.Rendered, "unattributed test output") {
		t.Fatalf("whitespace-only output test identity must be partial and not rendered: %#v", got)
	}
}

func TestParseGoTestJSONValidTestOutputIdentityRemainsComplete(t *testing.T) {
	stream := "{\"Action\":\"start\",\"Package\":\"probe/ok\"}\\n" +
		"{\"Action\":\"run\",\"Package\":\"probe/ok\",\"Test\":\"TestOK\"}\\n" +
		"{\"Action\":\"output\",\"Package\":\"probe/ok\",\"Test\":\"TestOK\",\"Output\":\"test output\\n\"}\\n" +
		"{\"Action\":\"pass\",\"Package\":\"probe/ok\",\"Test\":\"TestOK\"}\\n" +
		"{\"Action\":\"pass\",\"Package\":\"probe/ok\"}\\n"
	got := parseGoTestJSON(stream, false)
	if got.Confidence != verification.ParseComplete || !strings.Contains(got.Rendered, "test output") {
		t.Fatalf("valid test output must remain complete: %#v", got)
	}
}
