package execution

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func TestIsSensitivePath(t *testing.T) {
	sensitive := []string{
		".env",
		".env.local",
		"config/.env.production",
		".git/config",
		".ssh/id_rsa",
		"certs/server.pem",
		"certs/server.key",
		"config/credentials.json",
		"config/secrets.yaml",
		".npmrc",
		"gradle.properties",
	}
	for _, path := range sensitive {
		if !IsSensitivePath(path) {
			t.Fatalf("expected sensitive path: %s", path)
		}
	}

	safe := []string{
		"README.md",
		"core/execution/runtime.go",
		"src/environment.ts",
		"docs/security.md",
	}
	for _, path := range safe {
		if IsSensitivePath(path) {
			t.Fatalf("expected safe path: %s", path)
		}
	}
}

func TestPrepareSafeReadOnlyViewExcludesSecretsAndSymlinks(t *testing.T) {
	root := t.TempDir()
	view := t.TempDir()

	write := func(relative, content string) {
		path := filepath.Join(root, relative)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	write("src/app.go", "package main")
	write(".env", "TOKEN=secret")
	write("config/credentials.json", "{}")
	write("certs/server.pem", "secret pem")

	if err := os.Symlink(filepath.Join(root, ".env"), filepath.Join(root, "linked-secret")); err != nil && !os.IsPermission(err) {
		t.Fatal(err)
	}

	report, err := PrepareSafeReadOnlyView(root, view)
	if err != nil {
		t.Fatalf("PrepareSafeReadOnlyView() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(view, "src", "app.go")); err != nil {
		t.Fatalf("expected safe file: %v", err)
	}
	for _, relative := range []string{".env", "config/credentials.json", "certs/server.pem", "linked-secret"} {
		if _, err := os.Stat(filepath.Join(view, filepath.FromSlash(relative))); !os.IsNotExist(err) {
			t.Fatalf("sensitive path copied into safe view: %s", relative)
		}
	}

	excluded := append([]string(nil), report.ExcludedPaths...)
	sort.Strings(excluded)
	if len(excluded) < 3 {
		t.Fatalf("excluded paths = %#v", excluded)
	}
}

func TestReadOnlyEnvelopeValidationFailsClosed(t *testing.T) {
	base := readOnlyUnit()

	cases := []struct {
		name  string
		apply func(*WorkUnit)
	}{
		{"unknown source permission", func(unit *WorkUnit) { unit.Permissions.SourceCode = "mystery" }},
		{"unknown test permission", func(unit *WorkUnit) { unit.Permissions.Tests = "mystery" }},
		{"ask git commit", func(unit *WorkUnit) { unit.Permissions.GitCommit = "ask" }},
		{"read secrets", func(unit *WorkUnit) { unit.Permissions.Secrets = "read" }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			unit := base
			tc.apply(&unit)
			if err := validateReadOnlyUnit(unit); err == nil {
				t.Fatal("expected fail-closed permission validation")
			}
		})
	}
}

func TestSensitivePathClassificationStable(t *testing.T) {
	got := []bool{
		IsSensitivePath(".env"),
		IsSensitivePath("src/app.go"),
		IsSensitivePath(".git/config"),
	}
	want := []bool{true, false, true}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("classification = %#v, want %#v", got, want)
	}
}
