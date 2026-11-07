//go:build windows

package promotion

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestEnsureSafeParentRejectsWindowsJunction(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "project")
	outside := filepath.Join(base, "outside")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	junction := filepath.Join(root, "junction")
	output, err := exec.Command("cmd", "/c", "mklink", "/J", junction, outside).CombinedOutput()
	if err != nil {
		t.Skipf("junction creation unavailable on this runner: %v: %s", err, output)
	}
	if err := ensureSafeParent(root, "junction/payload.go", true); err == nil {
		t.Fatal("Windows junction parent must be rejected")
	}
	if _, err := os.Stat(filepath.Join(outside, "payload.go")); !os.IsNotExist(err) {
		t.Fatalf("outside path unexpectedly modified: %v", err)
	}
}
