//go:build !windows

package baseline

import (
	"context"
	"path/filepath"
	"syscall"
	"testing"
)

func TestSpecialFilesAreExcludedFromTheOriginalAndRejectedInTheStagedWorkspace(t *testing.T) {
	fx := newFixture(t, func(original string) {
		if err := syscall.Mkfifo(filepath.Join(original, "pipe.go"), 0o600); err != nil {
			t.Skipf("named pipes unavailable: %v", err)
		}
	})

	provider := fx.provider(t, &fakeVerifier{report: sampleReport()})
	if _, err := provider.Baseline(context.Background(), fx.staged); err != nil {
		t.Fatalf("a named pipe in the original must be excluded, not opened: %v", err)
	}

	if err := syscall.Mkfifo(filepath.Join(fx.staged, "events.go"), 0o600); err != nil {
		t.Skipf("named pipes unavailable: %v", err)
	}
	_, err := fx.provider(t, &fakeVerifier{report: sampleReport()}).Baseline(context.Background(), fx.staged)
	requireIntegrity(t, err, KindStagedInvalid)
}
