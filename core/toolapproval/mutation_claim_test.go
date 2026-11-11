package toolapproval

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

func TestMutationClaimIsIrreversibleAndScoped(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	digest := DigestRequest([]byte("approved-github-request"))
	if err := store.ClaimMutation("run-1", "task-1", "github", "create-pull-request", digest); err != nil {
		t.Fatal(err)
	}
	// Even after process exit there is no safe automatic replay.
	if err := store.ClaimMutation("run-1", "task-1", "github", "create-pull-request", digest); !errors.Is(err, ErrMutationAlreadyClaimed) {
		t.Fatalf("same mutation must be rejected, got %v", err)
	}
	if err := store.ClaimMutation("run-1", "task-2", "github", "create-pull-request", digest); err != nil {
		t.Fatalf("different task should not collide: %v", err)
	}
	if err := store.ClaimMutation("run-1", "task-1", "github", "create-pull-request", DigestRequest([]byte("different request"))); err != nil {
		t.Fatalf("different digest should not collide: %v", err)
	}
}

func TestMutationClaimConcurrentProcessesHaveOneWinner(t *testing.T) {
	root := t.TempDir()
	digest := DigestRequest([]byte("race-on-side-effect"))
	const attempts = 32
	var winners atomic.Int32
	var wg sync.WaitGroup
	errorsSeen := make(chan error, attempts)
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Independent stores simulate concurrent CLI processes.
			store, err := NewStore(root)
			if err != nil {
				errorsSeen <- err
				return
			}
			err = store.ClaimMutation("run-1", "task-1", "github", "create-pull-request", digest)
			if err == nil {
				winners.Add(1)
			} else if !errors.Is(err, ErrMutationAlreadyClaimed) {
				errorsSeen <- err
			}
		}()
	}
	wg.Wait()
	close(errorsSeen)
	for err := range errorsSeen {
		t.Errorf("unexpected race error: %v", err)
	}
	if got := winners.Load(); got != 1 {
		t.Fatalf("expected one winner, got %d", got)
	}
}

func TestMutationClaimRejectsMalformedIdentityAndSymlink(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	digest := DigestRequest([]byte("request"))
	for _, tc := range []struct{ runID, taskID, digest string }{
		{"../escape", "task", digest},
		{"run", "../escape", digest},
		{"run", "task", "not-a-sha256"},
	} {
		if err := store.ClaimMutation(tc.runID, tc.taskID, "github", "create-pull-request", tc.digest); err == nil {
			t.Fatalf("unsafe identity accepted: %+v", tc)
		}
	}
	target := t.TempDir()
	if err := os.Symlink(target, filepath.Join(root, "linked-run")); err != nil {
		if os.IsPermission(err) {
			t.Skip("symlinks are not permitted in this environment")
		}
		t.Fatal(err)
	}
	if err := store.ClaimMutation("linked-run", "task", "github", "create-pull-request", digest); err == nil {
		t.Fatal("symlink run directory must be rejected")
	}
}
