package runcontrol

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const LeaseSchemaVersion = 1

type Lease struct {
	SchemaVersion int       `json:"schema_version"`
	RunID         string    `json:"run_id"`
	OwnerID       string    `json:"owner_id"`
	AcquiredAt    time.Time `json:"acquired_at"`
	HeartbeatAt   time.Time `json:"heartbeat_at"`
}

func (s *Store) AcquireLease(runID string, now time.Time) (Lease, error) {
	if s == nil {
		return Lease{}, errors.New("run control store is required")
	}
	if !safeToken(runID) {
		return Lease{}, errors.New("run id is unsafe for run lease")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	ownerID, err := newLeaseOwnerID()
	if err != nil {
		return Lease{}, err
	}
	lease := Lease{
		SchemaVersion: LeaseSchemaVersion,
		RunID:         runID,
		OwnerID:       ownerID,
		AcquiredAt:    now.UTC(),
		HeartbeatAt:   now.UTC(),
	}
	path := s.leasePath(runID)
	data, err := json.MarshalIndent(lease, "", "  ")
	if err != nil {
		return Lease{}, err
	}
	data = append(data, '\n')
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if os.IsExist(err) {
		existing, loadErr := s.LoadLease(runID)
		if loadErr != nil {
			return Lease{}, fmt.Errorf("run lease already exists and is invalid: %w", loadErr)
		}
		return Lease{}, fmt.Errorf(
			"run %s already has an active or stale lease owned by %s",
			runID,
			existing.OwnerID,
		)
	}
	if err != nil {
		return Lease{}, fmt.Errorf("create run lease: %w", err)
	}
	cleanup := true
	defer func() {
		_ = file.Close()
		if cleanup {
			_ = os.Remove(path)
		}
	}()
	if _, err := file.Write(data); err != nil {
		return Lease{}, err
	}
	if err := file.Sync(); err != nil {
		return Lease{}, err
	}
	if err := file.Close(); err != nil {
		return Lease{}, err
	}
	cleanup = false
	return lease, nil
}

func (s *Store) LoadLease(runID string) (Lease, error) {
	if s == nil {
		return Lease{}, errors.New("run control store is required")
	}
	if !safeToken(runID) {
		return Lease{}, errors.New("run id is unsafe for run lease")
	}
	return loadLeaseFile(s.leasePath(runID), runID)
}

func (s *Store) HeartbeatLease(runID, ownerID string, now time.Time) (Lease, error) {
	if s == nil {
		return Lease{}, errors.New("run control store is required")
	}
	lease, err := s.LoadLease(runID)
	if err != nil {
		return Lease{}, err
	}
	if strings.TrimSpace(ownerID) == "" || lease.OwnerID != ownerID {
		return Lease{}, errors.New("run lease owner mismatch")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	now = now.UTC()
	if now.Before(lease.HeartbeatAt) {
		return Lease{}, errors.New("run lease heartbeat moved backwards")
	}
	lease.HeartbeatAt = now
	if err := s.replaceLease(lease); err != nil {
		return Lease{}, err
	}
	return lease, nil
}

func (s *Store) ReleaseLease(runID, ownerID string) error {
	if s == nil {
		return errors.New("run control store is required")
	}
	lease, err := s.LoadLease(runID)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if strings.TrimSpace(ownerID) == "" || lease.OwnerID != ownerID {
		return errors.New("run lease owner mismatch")
	}
	return os.Remove(s.leasePath(runID))
}

func (s *Store) ClaimStaleLease(
	runID string,
	now time.Time,
	threshold time.Duration,
) (Lease, Lease, error) {
	if s == nil {
		return Lease{}, Lease{}, errors.New("run control store is required")
	}
	stale, previous, err := s.LeaseStale(runID, now, threshold)
	if err != nil {
		return Lease{}, Lease{}, err
	}
	if !stale {
		return Lease{}, Lease{}, fmt.Errorf("run %s lease is not stale", runID)
	}

	path := s.leasePath(runID)
	quarantine := filepath.Join(
		s.root,
		fmt.Sprintf(".%s.reclaim-%d", runID, time.Now().UTC().UnixNano()),
	)
	if err := os.Rename(path, quarantine); err != nil {
		return Lease{}, Lease{}, fmt.Errorf("capture stale run lease: %w", err)
	}
	restore := true
	defer func() {
		if restore {
			if _, err := os.Lstat(path); os.IsNotExist(err) {
				_ = os.Rename(quarantine, path)
			}
		}
	}()

	captured, err := loadLeaseFile(quarantine, runID)
	if err != nil {
		return Lease{}, Lease{}, fmt.Errorf("validate captured stale lease: %w", err)
	}
	if captured.OwnerID != previous.OwnerID ||
		!captured.HeartbeatAt.Equal(previous.HeartbeatAt) ||
		!captured.AcquiredAt.Equal(previous.AcquiredAt) {
		return Lease{}, Lease{}, errors.New("run lease changed during stale claim")
	}

	recovery, err := s.AcquireLease(runID, now)
	if err != nil {
		return Lease{}, Lease{}, fmt.Errorf("acquire recovery lease: %w", err)
	}
	if err := os.Remove(quarantine); err != nil {
		_ = s.ReleaseLease(runID, recovery.OwnerID)
		return Lease{}, Lease{}, fmt.Errorf("remove captured stale lease: %w", err)
	}
	restore = false
	return recovery, previous, nil
}

func (s *Store) LeaseStale(runID string, now time.Time, threshold time.Duration) (bool, Lease, error) {
	if threshold <= 0 {
		return false, Lease{}, errors.New("lease stale threshold must be positive")
	}
	lease, err := s.LoadLease(runID)
	if err != nil {
		return false, Lease{}, err
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	now = now.UTC()
	if now.Before(lease.HeartbeatAt) {
		return false, Lease{}, errors.New("lease heartbeat is in the future")
	}
	return now.Sub(lease.HeartbeatAt) > threshold, lease, nil
}

func (s *Store) replaceLease(lease Lease) error {
	if err := validateLease(lease, lease.RunID); err != nil {
		return err
	}
	path := s.leasePath(lease.RunID)
	parent := filepath.Dir(path)
	temp, err := os.CreateTemp(parent, ".lease-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	cleanup := true
	defer func() {
		_ = temp.Close()
		if cleanup {
			_ = os.Remove(tempPath)
		}
	}()
	if err := temp.Chmod(0o600); err != nil {
		return err
	}
	data, err := json.MarshalIndent(lease, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if _, err := temp.Write(data); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}

	current, err := s.LoadLease(lease.RunID)
	if err != nil {
		return err
	}
	if current.OwnerID != lease.OwnerID {
		return errors.New("run lease owner changed before heartbeat commit")
	}
	if err := os.Rename(tempPath, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func loadLeaseFile(path, runID string) (Lease, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return Lease{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return Lease{}, errors.New("run lease must be a regular file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Lease{}, err
	}
	var lease Lease
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&lease); err != nil {
		return Lease{}, err
	}
	if err := validateLease(lease, runID); err != nil {
		return Lease{}, err
	}
	return lease, nil
}

func validateLease(lease Lease, runID string) error {
	if lease.SchemaVersion != LeaseSchemaVersion {
		return fmt.Errorf("unsupported run lease schema version: %d", lease.SchemaVersion)
	}
	if !safeToken(lease.RunID) || lease.RunID != runID {
		return errors.New("run lease identity is invalid")
	}
	if !safeToken(lease.OwnerID) {
		return errors.New("run lease owner id is invalid")
	}
	if lease.AcquiredAt.IsZero() || lease.HeartbeatAt.IsZero() {
		return errors.New("run lease timestamps are required")
	}
	if lease.HeartbeatAt.Before(lease.AcquiredAt) {
		return errors.New("run lease heartbeat precedes acquisition")
	}
	return nil
}

func newLeaseOwnerID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate run lease owner id: %w", err)
	}
	return "lease-" + hex.EncodeToString(value[:]), nil
}

func (s *Store) leasePath(runID string) string {
	return filepath.Join(s.root, runID+".lease.json")
}


type LeaseSession struct {
	store   *Store
	lease   Lease
	ctx     context.Context
	cancel  context.CancelFunc
	done     chan struct{}
	loopDone chan struct{}
	once     sync.Once
	errMu   sync.Mutex
	err     error
}

func StartLeaseSession(
	parent context.Context,
	store *Store,
	runID string,
	heartbeatInterval time.Duration,
) (*LeaseSession, error) {
	if store == nil {
		return nil, errors.New("run control store is required")
	}
	if heartbeatInterval <= 0 {
		heartbeatInterval = 2 * time.Second
	}
	if heartbeatInterval < 100*time.Millisecond {
		heartbeatInterval = 100 * time.Millisecond
	}
	lease, err := store.AcquireLease(runID, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(parent)
	session := &LeaseSession{
		store:  store,
		lease:  lease,
		ctx:    ctx,
		cancel: cancel,
		done:     make(chan struct{}),
		loopDone: make(chan struct{}),
	}
	go session.heartbeatLoop(heartbeatInterval)
	return session, nil
}

func (s *LeaseSession) Context() context.Context {
	if s == nil {
		return context.Background()
	}
	return s.ctx
}

func (s *LeaseSession) OwnerID() string {
	if s == nil {
		return ""
	}
	return s.lease.OwnerID
}

func (s *LeaseSession) Err() error {
	if s == nil {
		return nil
	}
	s.errMu.Lock()
	defer s.errMu.Unlock()
	return s.err
}

func (s *LeaseSession) Stop() error {
	if s == nil {
		return nil
	}
	s.once.Do(func() {
		close(s.done)
		s.cancel()

		// Release only after the heartbeat goroutine has stopped. Otherwise a
		// ticker that fired concurrently with Stop can race ReleaseLease and
		// try to renew a lease file that was just removed.
		<-s.loopDone

		releaseErr := s.store.ReleaseLease(s.lease.RunID, s.lease.OwnerID)
		if releaseErr != nil {
			s.setErr(releaseErr)
		}
	})
	return s.Err()
}

func (s *LeaseSession) heartbeatLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	defer close(s.loopDone)
	for {
		select {
		case <-s.done:
			return
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			if _, err := s.store.HeartbeatLease(
				s.lease.RunID,
				s.lease.OwnerID,
				time.Now().UTC(),
			); err != nil {
				s.setErr(fmt.Errorf("renew run lease: %w", err))
				s.cancel()
				return
			}
		}
	}
}

func (s *LeaseSession) setErr(err error) {
	if err == nil {
		return
	}
	s.errMu.Lock()
	defer s.errMu.Unlock()
	if s.err == nil {
		s.err = err
	}
}
