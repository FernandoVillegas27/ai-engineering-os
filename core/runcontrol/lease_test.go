package runcontrol

import (
	"context"
	"fmt"
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


func TestLeaseSessionHeartbeatsAndReleasesOnStop(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := StartLeaseSession(context.Background(), store, "run-session", 100*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Stop()
	first, err := store.LoadLease("run-session")
	if err != nil {
		t.Fatal(err)
	}
	second := waitForHeartbeatAdvance(t, store, "run-session", first.HeartbeatAt, 2*time.Second)
	if !second.HeartbeatAt.After(first.HeartbeatAt) {
		t.Fatalf("heartbeat did not advance: first=%#v second=%#v", first, second)
	}
	if err := session.Stop(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadLease("run-session"); !os.IsNotExist(err) {
		t.Fatalf("lease not released: %v", err)
	}
}

func TestLeaseSessionCancelsContextWhenLeaseDisappears(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	session, err := StartLeaseSession(context.Background(), store, "run-lost", 100*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Stop()

	if err := os.Remove(filepath.Join(root, "run-lost.lease.json")); err != nil {
		t.Fatal(err)
	}
	select {
	case <-session.Context().Done():
	case <-time.After(time.Second):
		t.Fatal("lease session did not cancel after losing lease")
	}
	if session.Err() == nil {
		t.Fatal("expected lease renewal error")
	}
}

func TestLeaseSessionRejectsSecondActiveOwner(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	first, err := StartLeaseSession(context.Background(), store, "run-exclusive", 100*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Stop()
	if _, err := StartLeaseSession(context.Background(), store, "run-exclusive", 100*time.Millisecond); err == nil {
		t.Fatal("expected second active lease rejection")
	}
}


func TestClaimStaleLeaseReplacesOwnerAndPreservesPreviousEvidence(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	previous, err := store.AcquireLease("run-recover", now.Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	recovery, captured, err := store.ClaimStaleLease(
		"run-recover",
		now,
		30*time.Second,
	)
	if err != nil {
		t.Fatal(err)
	}
	if captured.OwnerID != previous.OwnerID || recovery.OwnerID == previous.OwnerID {
		t.Fatalf("previous=%#v captured=%#v recovery=%#v", previous, captured, recovery)
	}
	current, err := store.LoadLease("run-recover")
	if err != nil {
		t.Fatal(err)
	}
	if current.OwnerID != recovery.OwnerID {
		t.Fatalf("current=%#v recovery=%#v", current, recovery)
	}
	if err := store.ReleaseLease("run-recover", recovery.OwnerID); err != nil {
		t.Fatal(err)
	}
}

func TestClaimStaleLeaseRejectsLiveLease(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	lease, err := store.AcquireLease("run-live", now.Add(-time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.ClaimStaleLease("run-live", now, 30*time.Second); err == nil {
		t.Fatal("expected live lease recovery rejection")
	}
	current, err := store.LoadLease("run-live")
	if err != nil {
		t.Fatal(err)
	}
	if current.OwnerID != lease.OwnerID {
		t.Fatalf("live lease changed: current=%#v original=%#v", current, lease)
	}
}

func TestOldLeaseOwnerCannotHeartbeatAfterStaleClaim(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	old, err := store.AcquireLease("run-fenced", now.Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	recovery, _, err := store.ClaimStaleLease("run-fenced", now, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer store.ReleaseLease("run-fenced", recovery.OwnerID)
	if _, err := store.HeartbeatLease("run-fenced", old.OwnerID, now.Add(time.Second)); err == nil {
		t.Fatal("expected old owner heartbeat rejection after recovery claim")
	}
}

func waitForHeartbeatAdvance(
	t *testing.T,
	store *Store,
	runID string,
	after time.Time,
	timeout time.Duration,
) Lease {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		lease, err := store.LoadLease(runID)
		if err == nil && lease.HeartbeatAt.After(after) {
			return lease
		}
		if err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	lease, err := store.LoadLease(runID)
	if err != nil {
		t.Fatal(err)
	}
	t.Fatalf("heartbeat did not advance within %s: %#v", timeout, lease)
	return Lease{}
}

func TestLeaseSessionRepeatedHeartbeatStopSoak(t *testing.T) {
	if testing.Short() {
		t.Skip("soak-style lease lifecycle test")
	}
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 8; i++ {
		runID := fmt.Sprintf("run-session-soak-%d", i)
		session, err := StartLeaseSession(context.Background(), store, runID, 100*time.Millisecond)
		if err != nil {
			t.Fatal(err)
		}
		first, err := store.LoadLease(runID)
		if err != nil {
			t.Fatal(err)
		}
		second := waitForHeartbeatAdvance(t, store, runID, first.HeartbeatAt, 2*time.Second)
		third := waitForHeartbeatAdvance(t, store, runID, second.HeartbeatAt, 2*time.Second)
		if !third.HeartbeatAt.After(second.HeartbeatAt) {
			t.Fatalf("heartbeat stopped advancing: second=%#v third=%#v", second, third)
		}
		if err := session.Stop(); err != nil {
			t.Fatal(err)
		}
		if _, err := store.LoadLease(runID); !os.IsNotExist(err) {
			t.Fatalf("lease %s not released: %v", runID, err)
		}
	}
}
