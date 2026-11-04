package runcontrol

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const SchemaVersion = 1

type CancelRequest struct {
	SchemaVersion int       `json:"schema_version"`
	RunID         string    `json:"run_id"`
	RequestedAt   time.Time `json:"requested_at"`
}

type Store struct {
	root string
}

func NewStore(root string) (*Store, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, errors.New("run control store root is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve run control root: %w", err)
	}
	if err := os.MkdirAll(absolute, 0o700); err != nil {
		return nil, fmt.Errorf("create run control root: %w", err)
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, errors.New("run control root must be a regular directory")
	}
	return &Store{root: absolute}, nil
}

func (s *Store) RequestCancel(runID string, now time.Time) (CancelRequest, error) {
	if s == nil {
		return CancelRequest{}, errors.New("run control store is required")
	}
	if !safeToken(runID) {
		return CancelRequest{}, errors.New("run id is unsafe for run control")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	request := CancelRequest{
		SchemaVersion: SchemaVersion,
		RunID:         runID,
		RequestedAt:   now.UTC(),
	}
	path := s.cancelPath(runID)
	data, err := json.MarshalIndent(request, "", "  ")
	if err != nil {
		return CancelRequest{}, err
	}
	data = append(data, '\n')
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if os.IsExist(err) {
		return s.LoadCancel(runID)
	}
	if err != nil {
		return CancelRequest{}, fmt.Errorf("persist cancel request: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return CancelRequest{}, err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return CancelRequest{}, err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return CancelRequest{}, err
	}
	return request, nil
}

func (s *Store) LoadCancel(runID string) (CancelRequest, error) {
	if s == nil {
		return CancelRequest{}, errors.New("run control store is required")
	}
	if !safeToken(runID) {
		return CancelRequest{}, errors.New("run id is unsafe for run control")
	}
	path := s.cancelPath(runID)
	info, err := os.Lstat(path)
	if err != nil {
		return CancelRequest{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return CancelRequest{}, errors.New("cancel request must be a regular file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return CancelRequest{}, err
	}
	var request CancelRequest
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return CancelRequest{}, err
	}
	if request.SchemaVersion != SchemaVersion || request.RunID != runID || request.RequestedAt.IsZero() {
		return CancelRequest{}, errors.New("cancel request is invalid")
	}
	return request, nil
}

func (s *Store) CancelRequested(runID string) (bool, error) {
	_, err := s.LoadCancel(runID)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) Clear(runID string) error {
	if s == nil {
		return errors.New("run control store is required")
	}
	if !safeToken(runID) {
		return errors.New("run id is unsafe for run control")
	}
	err := os.Remove(s.cancelPath(runID))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func (s *Store) cancelPath(runID string) string {
	return filepath.Join(s.root, runID+".cancel.json")
}

func WithCancellationWatcher(
	parent context.Context,
	store *Store,
	runID string,
	poll time.Duration,
) (context.Context, func(), error) {
	if store == nil {
		return nil, nil, errors.New("run control store is required")
	}
	if !safeToken(runID) {
		return nil, nil, errors.New("run id is unsafe for cancellation watcher")
	}
	if poll <= 0 {
		poll = 250 * time.Millisecond
	}
	if poll < 50*time.Millisecond {
		poll = 50 * time.Millisecond
	}
	requested, err := store.CancelRequested(runID)
	if err != nil {
		return nil, nil, err
	}
	ctx, cancel := context.WithCancel(parent)
	if requested {
		cancel()
		return ctx, cancel, nil
	}

	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(poll)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				requested, err := store.CancelRequested(runID)
				if err != nil {
					// Store corruption must not silently cancel a run. The caller can
					// still observe it through explicit control/status commands.
					continue
				}
				if requested {
					cancel()
					return
				}
			}
		}
	}()
	stop := func() {
		select {
		case <-done:
		default:
			close(done)
		}
		cancel()
	}
	return ctx, stop, nil
}

func safeToken(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || value == "." || value == ".." || strings.ContainsAny(value, "/\\") {
		return false
	}
	for _, r := range value {
		if !(r == '-' || r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}
