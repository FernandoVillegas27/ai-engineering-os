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
	if got := strings.Join(runner.invocations[0].Args, " "); got != "test -json ./..." {
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


func TestVerifyProvidesIsolatedGoBuildCacheAndCleansIt(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/test\n\ngo 1.23\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	runner := &fakeRunner{}
	report, err := (Runtime{Runner: runner}).Verify(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != "passed" || len(runner.invocations) != 1 {
		t.Fatalf("report=%#v invocations=%d", report, len(runner.invocations))
	}

	goCache := envValue(runner.invocations[0].Env, "GOCACHE")
	if goCache == "" {
		t.Fatal("GOCACHE not provided")
	}
	rel, err := filepath.Rel(root, goCache)
	if err != nil {
		t.Fatal(err)
	}
	if rel == "." || !strings.HasPrefix(filepath.ToSlash(rel), "../") {
		t.Fatalf("GOCACHE=%q should be outside workspace %q", goCache, root)
	}
	if _, err := os.Stat(goCache); !os.IsNotExist(err) {
		t.Fatalf("GOCACHE should be removed after verification, stat err=%v", err)
	}
}

func TestSafeVerificationEnvironmentPreservesConfiguredGoModuleCache(t *testing.T) {
	t.Setenv("GOMODCACHE", filepath.Join(t.TempDir(), "mod-cache"))
	t.Setenv("GOPATH", filepath.Join(t.TempDir(), "gopath"))
	env := safeVerificationEnvironment()
	if got := envValue(env, "GOMODCACHE"); got == "" {
		t.Fatal("GOMODCACHE should be preserved when configured")
	}
	if got := envValue(env, "GOPATH"); got == "" {
		t.Fatal("GOPATH should be preserved when configured")
	}
}

func TestVerifyClassifiesMissingGoCacheAsInfrastructure(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/test\n\ngo 1.23\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	report, err := (Runtime{Runner: &fakeRunner{
		fail:   true,
		stderr: "build cache is required, but could not be located: GOCACHE is not defined and %LocalAppData% is not defined",
	}}).Verify(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	check := report.Checks[0]
	if check.FailureKind != "infrastructure" || check.Correctable {
		t.Fatalf("check=%#v", check)
	}
}

func envValue(env []string, key string) string {
	prefix := key + "="
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			return strings.TrimPrefix(entry, prefix)
		}
	}
	return ""
}
