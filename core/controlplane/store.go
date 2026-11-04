package controlplane

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/FernandoVillegas27/ai-engineering-os/core/fslock"
)

type EventStore struct {
	root string
}

func NewEventStore(root string) (*EventStore, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, errors.New("control-plane event storage root is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve control-plane event storage root: %w", err)
	}
	if err := os.MkdirAll(absolute, 0o700); err != nil {
		return nil, fmt.Errorf("create control-plane event storage root: %w", err)
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, errors.New("control-plane event storage root must be a regular directory")
	}
	return &EventStore{root: absolute}, nil
}

func (s *EventStore) Append(ctx context.Context, value Event) (Event, error) {
	if s == nil {
		return Event{}, errors.New("control-plane event store is required")
	}
	if err := ValidateEvent(value); err != nil {
		return Event{}, fmt.Errorf("validate control-plane event: %w", err)
	}
	if !safeToken(value.RunID) {
		return Event{}, errors.New("run id is unsafe for control-plane storage")
	}
	runDir, err := s.ensureRunDir(value.RunID)
	if err != nil {
		return Event{}, err
	}
	lock, err := fslock.Acquire(ctx, filepath.Join(runDir, ".append.lock"), fslock.Options{})
	if err != nil {
		return Event{}, fmt.Errorf("acquire control-plane append lock: %w", err)
	}
	defer lock.Release()

	events, err := loadEvents(runDir)
	if err != nil {
		return Event{}, err
	}
	value.Sequence = 1
	if len(events) > 0 {
		value.Sequence = events[len(events)-1].Sequence + 1
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return Event{}, err
	}
	data = append(data, '\n')
	filename := fmt.Sprintf("%020d-%s.json", value.Sequence, value.ID)
	path := filepath.Join(runDir, filename)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return Event{}, fmt.Errorf("persist control-plane event: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return Event{}, err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return Event{}, err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return Event{}, err
	}
	return value, nil
}

func (s *EventStore) List(runID string) ([]Event, error) {
	if s == nil {
		return nil, errors.New("control-plane event store is required")
	}
	if !safeToken(runID) {
		return nil, errors.New("run id is unsafe for control-plane storage")
	}
	runDir := filepath.Join(s.root, runID)
	info, err := os.Lstat(runDir)
	if os.IsNotExist(err) {
		return []Event{}, nil
	}
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, errors.New("control-plane run directory must be a regular directory")
	}
	return loadEvents(runDir)
}

func (s *EventStore) RunIDs() ([]string, error) {
	if s == nil {
		return nil, errors.New("control-plane event store is required")
	}
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, entry := range entries {
		name := entry.Name()
		if !safeToken(name) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("control-plane run entry must not be a symlink: %s", name)
		}
		if !info.IsDir() {
			continue
		}
		ids = append(ids, name)
	}
	sort.Strings(ids)
	return ids, nil
}

func (s *EventStore) ensureRunDir(runID string) (string, error) {
	runDir := filepath.Join(s.root, runID)
	if err := os.Mkdir(runDir, 0o700); err != nil && !os.IsExist(err) {
		return "", err
	}
	info, err := os.Lstat(runDir)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", errors.New("control-plane run directory must be a regular directory")
	}
	return runDir, nil
}

func loadEvents(runDir string) ([]Event, error) {
	entries, err := os.ReadDir(runDir)
	if err != nil {
		return nil, err
	}
	var events []Event
	for _, entry := range entries {
		if entry.Name() == ".append.lock" || entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return nil, fmt.Errorf("control-plane event entry is not regular: %s", entry.Name())
		}
		data, err := os.ReadFile(filepath.Join(runDir, entry.Name()))
		if err != nil {
			return nil, err
		}
		var value Event
		decoder := json.NewDecoder(strings.NewReader(string(data)))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&value); err != nil {
			return nil, fmt.Errorf("decode control-plane event %s: %w", entry.Name(), err)
		}
		if err := ValidateEvent(value); err != nil {
			return nil, fmt.Errorf("validate control-plane event %s: %w", entry.Name(), err)
		}
		if filenameSequence(entry.Name()) != value.Sequence {
			return nil, fmt.Errorf("control-plane event filename sequence mismatch: %s", entry.Name())
		}
		events = append(events, value)
	}
	sort.Slice(events, func(i, j int) bool { return events[i].Sequence < events[j].Sequence })
	for i, event := range events {
		expected := uint64(i + 1)
		if event.Sequence != expected {
			return nil, fmt.Errorf("control-plane event sequence gap or duplicate: got %d want %d", event.Sequence, expected)
		}
	}
	return events, nil
}

func filenameSequence(name string) uint64 {
	index := strings.IndexByte(name, '-')
	if index <= 0 {
		return 0
	}
	value, _ := strconv.ParseUint(name[:index], 10, 64)
	return value
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
