package localverify

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FernandoVillegas27/ai-engineering-os/integrations/commandmodel"
)

type fakeRunner struct {
	invocations []commandmodel.Invocation
	fail        bool
	stderr      string
}

func (f *fakeRunner) LookPath(file string) (string, error) { return file, nil }

func (f *fakeRunner) Run(_ context.Context, invocation commandmodel.Invocation) (string, string, error) {
	f.invocations = append(f.invocations, invocation)
	if f.fail {
		stderr := f.stderr
		if stderr == "" {
			stderr = "tests failed"
		}
		return "", stderr, os.ErrInvalid
	}
	return "ok", "", nil
}

func TestVerifyRunsGoTests(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/test\n\ngo 1.23\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{}
	report, err := (Runtime{Runner: runner}).Verify(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if string(report.Status) != "passed" || len(runner.invocations) != 1 {
		t.Fatalf("report=%#v invocations=%d", report, len(runner.invocations))
	}
	if got := strings.Join(runner.invocations[0].Args, " "); got != "test ./..." {
		t.Fatalf("args=%q", got)
	}
}

func TestVerifyReportsFailure(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/test\n\ngo 1.23\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	report, err := (Runtime{Runner: &fakeRunner{fail: true}}).Verify(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if string(report.Status) != "failed" || report.Checks[0].Passed {
		t.Fatalf("report=%#v", report)
	}
}

func TestPlanNodeVitest(t *testing.T) {
	root := t.TempDir()
	manifest := []byte(`{"scripts":{"test":"vitest"}}`)
	if err := os.WriteFile(filepath.Join(root, "package.json"), manifest, 0o600); err != nil {
		t.Fatal(err)
	}
	checks, err := planChecks(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(checks) != 1 || strings.Join(checks[0].args, " ") != "test -- --run" {
		t.Fatalf("checks=%#v", checks)
	}
}


func TestVerifyClassifiesInfrastructureFailure(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/test\n\ngo 1.23\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	report, err := (Runtime{Runner: &fakeRunner{
		fail:   true,
		stderr: "proxyconnect tcp: 407 Proxy Authentication Required",
	}}).Verify(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	check := report.Checks[0]
	if check.FailureKind != "infrastructure" || check.Correctable {
		t.Fatalf("check=%#v", check)
	}
}

func TestVerifyClassifiesTestFailureAsCorrectableCode(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/test\n\ngo 1.23\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	report, err := (Runtime{Runner: &fakeRunner{
		fail:   true,
		stderr: "FAIL example.com/test\n--- FAIL: TestValue",
	}}).Verify(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	check := report.Checks[0]
	if check.FailureKind != "code" || !check.Correctable {
		t.Fatalf("check=%#v", check)
	}
}
