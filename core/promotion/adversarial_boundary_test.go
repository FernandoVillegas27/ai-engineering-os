package promotion

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FernandoVillegas27/ai-engineering-os/core/staging"
)

// These tests exercise the promotion boundary with hostile filesystem inputs.
// They must not depend on model-provider availability or network access.
func TestOpenPinnedRegularFileRejectsSymlinkEscape(t *testing.T) {
	base := t.TempDir()
	outside := filepath.Join(base, "outside.txt")
	if err := os.WriteFile(outside, []byte("private outside content"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "untrusted-link.txt")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("creating symlinks is unavailable on this runner: %v", err)
	}

	file, _, err := openPinnedRegularFile(link)
	if file != nil {
		_ = file.Close()
		t.Fatal("unexpectedly opened symlink into untrusted path")
	}
	if err == nil {
		t.Fatal("symlink must be rejected before reading outside content")
	}
	data, err := os.ReadFile(outside)
	if err != nil || string(data) != "private outside content" {
		t.Fatalf("outside file changed: data=%q err=%v", data, err)
	}
}

func TestEnsureSafeParentRejectsSymlinkedParent(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "project")
	outside := filepath.Join(base, "outside")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "injected")); err != nil {
		t.Skipf("creating symlinks is unavailable on this runner: %v", err)
	}
	for _, create := range []bool{false, true} {
		if err := ensureSafeParent(root, "injected/payload.go", create); err == nil {
			t.Fatalf("symlink parent accepted with create=%v", create)
		}
	}
	if _, err := os.Stat(filepath.Join(outside, "payload.go")); !os.IsNotExist(err) {
		t.Fatalf("outside path unexpectedly created: %v", err)
	}
}

func TestVerifyTargetStateDetectsChangedContentAndSymlink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "main.go")
	before := []byte("package main\nconst v = 1\n")
	if err := os.WriteFile(target, before, 0o600); err != nil {
		t.Fatal(err)
	}
	preImage := staging.WorkspaceSnapshot{"main.go": sha256.Sum256(before)}
	change := staging.WorkspaceChange{Path: "main.go", Action: "modify"}
	if err := verifyTargetState(root, change, preImage); err != nil {
		t.Fatalf("unchanged target incorrectly rejected: %v", err)
	}
	if err := os.WriteFile(target, []byte("package main\nconst v = 999\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := verifyTargetState(root, change, preImage); err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatalf("changed content was not rejected: %v", err)
	}
	outside := filepath.Join(t.TempDir(), "outside.go")
	if err := os.WriteFile(outside, before, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, target); err != nil {
		t.Skipf("creating symlinks is unavailable on this runner: %v", err)
	}
	if err := verifyTargetState(root, change, preImage); err == nil {
		t.Fatal("symlink with expected content must not pass identity verification")
	}
}

func TestVerifyTargetStateRejectsUnplannedCreateCollision(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "new.go")
	change := staging.WorkspaceChange{Path: "new.go", Action: "create"}
	if err := verifyTargetState(root, change, staging.WorkspaceSnapshot{}); err != nil {
		t.Fatalf("missing target incorrectly rejected: %v", err)
	}
	if err := os.WriteFile(target, []byte("existing data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := verifyTargetState(root, change, staging.WorkspaceSnapshot{}); err == nil {
		t.Fatal("existing target must not be overwritten by create action")
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "existing data" {
		t.Fatalf("existing file was modified: %q err=%v", data, err)
	}
}


func TestEnsureSafeParentRejectsTraversalAbsoluteAndVolumePaths(t *testing.T) {
	root := t.TempDir()
	cases := []string{
		"../outside/payload.go",
		"nested/../../outside/payload.go",
		"/absolute/payload.go",
		"C:\\absolute\\payload.go",
		"\\\\server\\share\\payload.go",
	}
	for _, relative := range cases {
		t.Run(relative, func(t *testing.T) {
			if err := ensureSafeParent(root, relative, true); err == nil {
				t.Fatalf("unsafe path accepted: %q", relative)
			}
		})
	}
}
