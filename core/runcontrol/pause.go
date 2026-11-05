package runcontrol

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

type PauseRequest struct {
	SchemaVersion int       `json:"schema_version"`
	RunID         string    `json:"run_id"`
	RequestedAt   time.Time `json:"requested_at"`
}

func (s *Store) RequestPause(runID string, now time.Time) (PauseRequest, error) {
	if s == nil {
		return PauseRequest{}, errors.New("run control store is required")
	}
	if !safeToken(runID) {
		return PauseRequest{}, errors.New("run id is unsafe for run control")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	request := PauseRequest{
		SchemaVersion: SchemaVersion,
		RunID:         runID,
		RequestedAt:   now.UTC(),
	}
	path := s.pausePath(runID)
	data, err := json.MarshalIndent(request, "", "  ")
	if err != nil {
		return PauseRequest{}, err
	}
	data = append(data, '\n')
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if os.IsExist(err) {
		return s.LoadPause(runID)
	}
	if err != nil {
		return PauseRequest{}, fmt.Errorf("persist pause request: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return PauseRequest{}, err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return PauseRequest{}, err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return PauseRequest{}, err
	}
	return request, nil
}

func (s *Store) LoadPause(runID string) (PauseRequest, error) {
	if s == nil {
		return PauseRequest{}, errors.New("run control store is required")
	}
	if !safeToken(runID) {
		return PauseRequest{}, errors.New("run id is unsafe for run control")
	}
	path := s.pausePath(runID)
	info, err := os.Lstat(path)
	if err != nil {
		return PauseRequest{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return PauseRequest{}, errors.New("pause request must be a regular file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return PauseRequest{}, err
	}
	var request PauseRequest
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return PauseRequest{}, err
	}
	if request.SchemaVersion != SchemaVersion || request.RunID != runID || request.RequestedAt.IsZero() {
		return PauseRequest{}, errors.New("pause request is invalid")
	}
	return request, nil
}

func (s *Store) PauseRequested(runID string) (bool, error) {
	_, err := s.LoadPause(runID)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) ClearPause(runID string) error {
	if s == nil {
		return errors.New("run control store is required")
	}
	if !safeToken(runID) {
		return errors.New("run id is unsafe for run control")
	}
	err := os.Remove(s.pausePath(runID))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func (s *Store) pausePath(runID string) string {
	return s.root + string(os.PathSeparator) + runID + ".pause.json"
}
