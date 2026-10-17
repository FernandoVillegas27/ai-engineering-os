package doctor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckWorkspace(t *testing.T) {
	check := CheckWorkspace(t.TempDir())
	if check.Level != LevelOK {
		t.Fatalf("level = %q, want %q", check.Level, LevelOK)
	}
}

func TestCheckProjectContextWarnsWhenMissing(t *testing.T) {
	check := CheckProjectContext(t.TempDir())
	if check.Level != LevelWarn {
		t.Fatalf("level = %q, want %q", check.Level, LevelWarn)
	}
}

func TestCheckProjectContextOKWhenPresent(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".ai")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "project.yaml"), []byte("project:\n  id: demo\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	check := CheckProjectContext(root)
	if check.Level != LevelOK {
		t.Fatalf("level = %q, want %q", check.Level, LevelOK)
	}
}

func TestCheckEngramProject(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".engram")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "config.json"), []byte(`{"project_name":"ai-engineering-os"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	check := CheckEngramProject(root)
	if check.Level != LevelOK {
		t.Fatalf("level = %q, want %q", check.Level, LevelOK)
	}
}

func TestCheckEngramProjectWarnsWhenMissing(t *testing.T) {
	check := CheckEngramProject(t.TempDir())
	if check.Level != LevelWarn {
		t.Fatalf("level = %q, want %q", check.Level, LevelWarn)
	}
}

func TestReportHasErrors(t *testing.T) {
	report := Report{Checks: []Check{
		{Level: LevelOK},
		{Level: LevelError},
	}}

	if !report.HasErrors() {
		t.Fatal("expected report to contain errors")
	}
}
