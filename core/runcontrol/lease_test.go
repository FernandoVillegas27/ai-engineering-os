package runcontrol

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLeaseAcquireHeartbeatRelease(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Add(-time.Minute)
	lease, err := store.AcquireLease("run-lease", now)
	if err != nil {
		t.Fatal(err)
	}
	if lease.OwnerID == "" || lease.RunID != "run-lease" {
		t.Fatalf("lease=%#v", lease)
	}
	updated, err := store.HeartbeatLease("run-lease", lease.OwnerID, now.Add(10*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if !updated.HeartbeatAt.After(lease.HeartbeatAt) {
		t.Fatalf("updated=%#v original=%#v", updated, lease)
	}
	if err := store.ReleaseLease("run-lease", lease.OwnerID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadLease("run-lease"); !os.IsNotExist(err) {
		t.Fatalf("expected removed lease, err=%v", err)
	}
}

func TestLeaseRejectsConcurrentAcquireEvenWhenStale(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Add(-time.Hour)
	lease, err := store.AcquireLease("run-existing", now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AcquireLease("run-existing", time.Now().UTC()); err == nil ||
		!strings.Contains(err.Error(), lease.OwnerID) {
		t.Fatalf("expected existing lease rejection, err=%v", err)
	}
	stale, loaded, err := store.LeaseStale(
		"run-existing",
		time.Now().UTC(),
		30*time.Second,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !stale || loaded.OwnerID != lease.OwnerID {
		t.Fatalf("stale=%t loaded=%#v", stale, loaded)
	}
}

func TestLeaseRejectsOwnerMismatchAndBackwardHeartbeat(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Add(-time.Minute)
	lease, err := store.AcquireLease("run-owner", now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.HeartbeatLease("run-owner", "lease-wrong", now.Add(time.Second)); err == nil {
		t.Fatal("expected heartbeat owner mismatch")
	}
	if err := store.ReleaseLease("run-owner", "lease-wrong"); err == nil {
		t.Fatal("expected release owner mismatch")
	}
	if _, err := store.HeartbeatLease("run-owner", lease.OwnerID, now.Add(-time.Second)); err == nil {
		t.Fatal("expected backward heartbeat rejection")
	}
}

func TestLeaseStaleUsesHeartbeatNotAcquisition(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Add(-time.Hour)
	lease, err := store.AcquireLease("run-heartbeat", now)
	if err != nil {
		t.Fatal(err)
	}
	heartbeatAt := time.Now().UTC().Add(-5 * time.Second)
	if _, err := store.HeartbeatLease("run-heartbeat", lease.OwnerID, heartbeatAt); err != nil {
		t.Fatal(err)
	}
	stale, _, err := store.LeaseStale(
		"run-heartbeat",
		time.Now().UTC(),
		30*time.Second,
	)
	if err != nil {
		t.Fatal(err)
	}
	if stale {
		t.Fatal("recent heartbeat should keep old lease live")
	}
}

func TestLeaseRejectsSymlinkEntry(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "target.json")
	if err := os.WriteFile(target, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "run-link.lease.json")
	if err := os.Symlink(target, link); err != nil {
		if os.IsPermission(err) {
			t.Skip("symlink creation not permitted")
		}
		t.Fatal(err)
	}
	if _, err := store.LoadLease("run-link"); err == nil {
		t.Fatal("expected symlink lease rejection")
	}
}

func TestLeaseRejectsInvalidThresholdAndFutureHeartbeat(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	future := time.Now().UTC().Add(time.Minute)
	if _, err := store.AcquireLease("run-future", future); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.LeaseStale("run-future", time.Now().UTC(), 0); err == nil {
		t.Fatal("expected invalid threshold rejection")
	}
	if _, _, err := store.LeaseStale("run-future", time.Now().UTC(), time.Second); err == nil {
		t.Fatal("expected future heartbeat rejection")
	}
}
