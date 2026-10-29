package execution

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"syscall"
	"testing"
)

// errPrivilegeNotHeld is ERROR_PRIVILEGE_NOT_HELD, returned by Windows when symlinks are not allowed.
const errPrivilegeNotHeld = syscall.Errno(1314)

// requireSymlink skips only when Windows denies symlink privileges; any other failure is fatal.
func requireSymlink(t *testing.T, target, link string) {
	t.Helper()
	err := os.Symlink(target, link)
	if err == nil {
		return
	}
	if runtime.GOOS == "windows" && errors.Is(err, errPrivilegeNotHeld) {
		t.Skipf("symlink creation requires a privilege this user does not hold: %v", err)
	}
	t.Fatal(err)
}

func mustPin(t *testing.T, path string) pinnedFileInfo {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	pinned, err := pinFileIdentity(info)
	if err != nil {
		t.Fatal(err)
	}
	return pinned
}

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
		"config/id_ecdsa",
		"config/service-account.json",
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
	write("config/id_ecdsa", "private key")
	write("config/service-account.json", "{}")
	write("assets/logo.png", "binary-ish")

	report, err := PrepareSafeReadOnlyView(root, view)
	if err != nil {
		t.Fatalf("PrepareSafeReadOnlyView() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(view, "src", "app.go")); err != nil {
		t.Fatalf("expected safe file: %v", err)
	}
	for _, relative := range []string{".env", "config/credentials.json", "certs/server.pem", "config/id_ecdsa", "config/service-account.json", "assets/logo.png"} {
		if _, err := os.Stat(filepath.Join(view, filepath.FromSlash(relative))); !os.IsNotExist(err) {
			t.Fatalf("sensitive path copied into safe view: %s", relative)
		}
	}

	excluded := append([]string(nil), report.ExcludedPaths...)
	sort.Strings(excluded)
	if len(excluded) < 3 {
		t.Fatalf("excluded paths = %#v", excluded)
	}

	t.Run("symlink", func(t *testing.T) {
		linkRoot := t.TempDir()
		linkView := t.TempDir()
		if err := os.WriteFile(filepath.Join(linkRoot, ".env"), []byte("TOKEN=secret"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(linkRoot, "app.go"), []byte("package main"), 0o600); err != nil {
			t.Fatal(err)
		}
		requireSymlink(t, filepath.Join(linkRoot, ".env"), filepath.Join(linkRoot, "linked-secret"))

		linkReport, err := PrepareSafeReadOnlyView(linkRoot, linkView)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Lstat(filepath.Join(linkView, "linked-secret")); !os.IsNotExist(err) {
			t.Fatal("symlink copied into safe view")
		}
		if !reflect.DeepEqual(linkReport.ExcludedPaths, []string{".env", "linked-secret"}) {
			t.Fatalf("excluded paths = %#v", linkReport.ExcludedPaths)
		}
	})
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


func TestPositiveRuntimeFilePolicyExcludesUnknownFiles(t *testing.T) {
	policy := DefaultSafeViewPolicy()

	allowed := []string{
		"src/app.go",
		"web/app.ts",
		"pom.xml",
		"package.json",
		"docs/security.md",
	}
	for _, path := range allowed {
		if !IsAllowedRuntimeFile(path, policy) {
			t.Fatalf("expected allowed runtime file: %s", path)
		}
	}

	blocked := []string{
		"assets/logo.png",
		"config/id_ecdsa",
		"bin/tool.exe",
		"archive/data.bin",
	}
	for _, path := range blocked {
		if IsAllowedRuntimeFile(path, policy) {
			t.Fatalf("unexpected runtime file allowed: %s", path)
		}
	}
}

func TestCustomSafeViewPolicyCanAuthorizeAdditionalExtension(t *testing.T) {
	policy := DefaultSafeViewPolicy()
	policy.AllowedExtensions[".tf"] = true
	if !IsAllowedRuntimeFile("infra/main.tf", policy) {
		t.Fatal("expected custom extension to be allowed")
	}
}


func TestCopyVerifiedRegularFileRejectsIdentityChange(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.go")
	target := filepath.Join(root, "copy.go")

	if err := os.WriteFile(source, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	expected := mustPin(t, source)

	replacement := filepath.Join(root, "replacement.go")
	if err := os.WriteFile(replacement, []byte("replacement"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, source); err != nil {
		t.Fatal(err)
	}

	if err := copyVerifiedRegularFile(source, target, expected); err == nil {
		t.Fatal("expected identity-change rejection")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("target must not be created after identity mismatch")
	}
}

func TestCopyVerifiedRegularFileRejectsReplacementWithIdenticalContent(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.go")
	target := filepath.Join(root, "copy.go")
	if err := os.WriteFile(source, []byte("same bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	expected := mustPin(t, source)

	// Keep the original alive: deleting it would let the OS reuse its inode or file index for the replacement.
	if err := os.Rename(source, filepath.Join(root, "original.go")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("same bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	original := mustPin(t, filepath.Join(root, "original.go"))
	if os.SameFile(original.FileInfo, mustPin(t, source).FileInfo) {
		t.Fatal("test precondition: the replacement must have a distinct identity")
	}

	if err := copyVerifiedRegularFile(source, target, expected); err == nil {
		t.Fatal("identity must be checked independently of content")
	}
}

func TestCopyVerifiedRegularFileRejectsHardLinkReplacement(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.go")
	other := filepath.Join(root, "other.go")
	target := filepath.Join(root, "copy.go")
	if err := os.WriteFile(source, []byte("safe"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(other, []byte("other"), 0o600); err != nil {
		t.Fatal(err)
	}
	expected := mustPin(t, source)

	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(other, source); err != nil {
		t.Skipf("hard links unavailable: %v", err)
	}

	if err := copyVerifiedRegularFile(source, target, expected); err == nil {
		t.Fatal("expected hard-link replacement rejection")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("target must not be created for a replaced file")
	}
}

func TestCopyVerifiedRegularFileCopiesUnchangedFile(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.go")
	target := filepath.Join(root, "copy.go")
	if err := os.WriteFile(source, []byte("payload"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := copyVerifiedRegularFile(source, target, mustPin(t, source)); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(target)
	if err != nil || string(got) != "payload" {
		t.Fatalf("copy = %q, %v", got, err)
	}
}

// Documents why pinning is required: on Windows an unpinned Lstat result follows a later replacement.
func TestUnpinnedLstatDoesNotDetectReplacementOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("lazy file identity is Windows-specific")
	}
	root := t.TempDir()
	source := filepath.Join(root, "source.go")
	if err := os.WriteFile(source, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	unpinned, err := os.Lstat(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("replacement"), 0o600); err != nil {
		t.Fatal(err)
	}
	current, err := os.Lstat(source)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(unpinned, current) {
		t.Fatal("premise changed: Go now resolves Lstat identity eagerly on Windows; pinning may be redundant")
	}
}

func TestPinFileIdentityFailsClosedWhenFileDisappearsOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("lazy file identity is Windows-specific")
	}
	source := filepath.Join(t.TempDir(), "source.go")
	if err := os.WriteFile(source, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}
	if _, err := pinFileIdentity(info); err == nil {
		t.Fatal("expected pinning to fail when identity cannot be resolved")
	}
}

func TestCopyVerifiedRegularFileRejectsSymlinkReplacement(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.go")
	secret := filepath.Join(root, "secret.txt")
	target := filepath.Join(root, "copy.go")

	if err := os.WriteFile(source, []byte("safe"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secret, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	expected := mustPin(t, source)
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}
	requireSymlink(t, secret, source)

	if err := copyVerifiedRegularFile(source, target, expected); err == nil {
		t.Fatal("expected symlink replacement rejection")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("target must not be created for symlink replacement")
	}
}
