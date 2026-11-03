package localverify

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FernandoVillegas27/ai-engineering-os/core/baseline"
	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/quality"
	"github.com/FernandoVillegas27/ai-engineering-os/core/staging"
	"github.com/FernandoVillegas27/ai-engineering-os/core/verification"
)

func TestBaselineOfARealGoProjectSeparatesPreexistingFromNewFailures(t *testing.T) {
	if testing.Short() {
		t.Skip("runs real go test processes")
	}
	goBinary, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go toolchain not available")
	}
	cache, err := exec.Command(goBinary, "env", "GOCACHE").Output()
	if err != nil {
		t.Skipf("cannot resolve GOCACHE: %v", err)
	}

	base := t.TempDir()
	original := filepath.Join(base, "project")
	staged := filepath.Join(base, "stage")
	for name, content := range map[string]string{
		"go.mod":           "module example.com/integ\n\ngo 1.23\n",
		"calc/calc.go":     "package calc\n\nfunc Add(a, b int) int { return a + b }\n",
		"calc/calc_test.go": "package calc\n\nimport (\n\t\"os\"\n\t\"testing\"\n)\n\nfunc TestAdd(t *testing.T) {\n\tif Add(1, 2) != 3 {\n\t\tt.Fatal(\"wrong sum\")\n\t}\n}\n\n// TestOld fails before any change and prints the workspace location.\nfunc TestOld(t *testing.T) {\n\twd, _ := os.Getwd()\n\tt.Fatalf(\"known failure in %s\", wd)\n}\n",
	} {
		path := filepath.Join(original, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	if err := os.MkdirAll(staged, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := execution.PrepareSafeReadOnlyView(original, staged); err != nil {
		t.Fatal(err)
	}
	snapshot, err := staging.SnapshotWorkspace(staged, execution.DefaultSafeViewPolicy())
	if err != nil {
		t.Fatal(err)
	}
	preImage := staging.NewPreImage(snapshot)

	// The "agent" adds an authorized test that fails.
	newTest := "package calc\n\nimport \"testing\"\n\nfunc TestNew(t *testing.T) { t.Fatal(\"introduced by the change\") }\n"
	if err := os.WriteFile(filepath.Join(staged, "calc", "new_test.go"), []byte(newTest), 0o600); err != nil {
		t.Fatal(err)
	}

	runtime := Runtime{Runner: goCacheRunner{cache: strings.TrimSpace(string(cache))}}
	stagedReport, err := runtime.Verify(context.Background(), staged)
	if err != nil {
		t.Fatal(err)
	}

	provider, err := baseline.New(baseline.Config{OriginalRoot: original, PreImage: preImage, Authorized: []string{"calc/new_test.go"}, Verifier: runtime})
	if err != nil {
		t.Fatal(err)
	}
	result, err := provider.Baseline(context.Background(), staged)
	if err != nil {
		t.Fatal(err)
	}
	if result.Report.Checks[0].Passed || result.Report.Checks[0].FailureParse != verification.ParseComplete {
		t.Fatalf("baseline check=%#v", result.Report.Checks[0])
	}

	comparison := quality.Compare(result.Report, stagedReport)
	if len(comparison.Preexisting) != 1 || comparison.Preexisting[0].Failure.ID != "example.com/integ/calc#TestOld" {
		t.Fatalf("TestOld fails in both workspaces with a path in its message and must stay preexisting: %#v", comparison)
	}
	if len(comparison.Regressions) != 1 || comparison.Regressions[0].Failure.ID != "example.com/integ/calc#TestNew" {
		t.Fatalf("regressions=%#v", comparison.Regressions)
	}
	if len(comparison.ChangedCause) != 0 || len(comparison.Unclassified) != 0 || len(comparison.Blocking) != 0 {
		t.Fatalf("comparison=%#v", comparison)
	}
}
