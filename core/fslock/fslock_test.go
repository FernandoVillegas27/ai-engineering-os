package fslock

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAcquireAndRelease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lock")
	lock, err := Acquire(context.Background(), path, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	if err := lock.Release(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("lock should be removed, err=%v", err)
	}
}

func TestAcquireHonorsCancellationWhenLockIsFresh(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lock")
	first, err := Acquire(context.Background(), path, Options{StaleAfter: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	defer first.Release()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = Acquire(ctx, path, Options{RetryInterval: time.Millisecond, StaleAfter: time.Hour})
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%#v", err)
	}
}

func TestAcquireReclaimsStaleLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lock")
	if err := os.WriteFile(path, []byte("stale-owner\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-10 * time.Minute)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}

	lock, err := Acquire(context.Background(), path, Options{StaleAfter: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Release()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) == "stale-owner\n" {
		t.Fatal("stale lock was not replaced")
	}
}

func TestReleaseDoesNotRemoveAnotherOwnersReplacement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lock")
	lock, err := Acquire(context.Background(), path, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("another-owner\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := lock.Release(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "another-owner\n" {
		t.Fatalf("replacement lock changed: %q", data)
	}
}

func TestAcquireRejectsSymlinkLockPath(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.WriteFile(target, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "lock")
	if err := os.Symlink(target, link); err != nil {
		if os.IsPermission(err) {
			t.Skip("symlink creation not permitted")
		}
		t.Fatal(err)
	}
	_, err := Acquire(context.Background(), link, Options{StaleAfter: time.Second})
	if err == nil {
		t.Fatal("expected symlink lock rejection")
	}
}
