package runstate

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type Store struct {
	root string
	mu   sync.Mutex
}

func NewStore(root string) (*Store, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, errors.New("run-state storage root is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve run-state storage root: %w", err)
	}
	if err := os.MkdirAll(absolute, 0o700); err != nil {
		return nil, fmt.Errorf("create run-state storage root: %w", err)
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, errors.New("run-state storage root must be a regular directory")
	}
	return &Store{root: absolute}, nil
}

func (s *Store) Save(run Run) error {
	if s == nil {
		return errors.New("run-state store is required")
	}
	if err := Validate(run); err != nil {
		return fmt.Errorf("validate run state: %w", err)
	}
	if !safeID(run.ID) {
		return errors.New("run id is unsafe for storage")
	}

	data, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		return fmt.Errorf("encode run state: %w", err)
	}
	data = append(data, '\n')

	s.mu.Lock()
	defer s.mu.Unlock()

	target := filepath.Join(s.root, run.ID+".json")
	temp, err := os.CreateTemp(s.root, ".runstate-*.tmp")
	if err != nil {
		return fmt.Errorf("create run-state temp file: %w", err)
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
		return fmt.Errorf("protect run-state temp file: %w", err)
	}
	if _, err := temp.Write(data); err != nil {
		return fmt.Errorf("write run state: %w", err)
	}
	if err := temp.Sync(); err != nil {
		return fmt.Errorf("sync run state: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close run state: %w", err)
	}

	if err := atomicReplace(tempPath, target); err != nil {
		return fmt.Errorf("replace run state: %w", err)
	}
	cleanup = false
	return nil
}

func (s *Store) Load(id string) (Run, error) {
	if s == nil {
		return Run{}, errors.New("run-state store is required")
	}
	if !safeID(id) {
		return Run{}, errors.New("run id is unsafe for storage")
	}
	path := filepath.Join(s.root, id+".json")
	info, err := os.Lstat(path)
	if err != nil {
		return Run{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return Run{}, errors.New("run-state entry must be a regular file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Run{}, err
	}
	var run Run
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&run); err != nil {
		return Run{}, fmt.Errorf("decode run state: %w", err)
	}
	if err := Validate(run); err != nil {
		return Run{}, fmt.Errorf("validate persisted run state: %w", err)
	}
	if run.ID != id {
		return Run{}, errors.New("persisted run id does not match requested id")
	}
	return run, nil
}

func (s *Store) List() ([]Run, error) {
	if s == nil {
		return nil, errors.New("run-state store is required")
	}
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil, err
	}
	var runs []Run
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		if !safeID(id) {
			continue
		}
		run, err := s.Load(id)
		if err != nil {
			return nil, fmt.Errorf("load %s: %w", id, err)
		}
		runs = append(runs, run)
	}
	sortRuns(runs)
	return runs, nil
}

func safeID(id string) bool {
	id = strings.TrimSpace(id)
	if id == "" || strings.ContainsAny(id, "/\\") || id == "." || id == ".." {
		return false
	}
	for _, r := range id {
		if !(r == '-' || r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

func sortRuns(runs []Run) {
	for i := 1; i < len(runs); i++ {
		for j := i; j > 0 && runs[j].UpdatedAt.After(runs[j-1].UpdatedAt); j-- {
			runs[j], runs[j-1] = runs[j-1], runs[j]
		}
	}
}

func atomicReplace(source, target string) error {
	// os.Rename is atomic on the same filesystem on Unix. On Windows it cannot
	// replace an existing file, so move the previous version aside first and
	// restore it if the final rename fails.
	if _, err := os.Lstat(target); os.IsNotExist(err) {
		return os.Rename(source, target)
	} else if err != nil {
		return err
	}

	backup := target + ".previous"
	_ = os.Remove(backup)
	if err := os.Rename(target, backup); err != nil {
		return err
	}
	if err := os.Rename(source, target); err != nil {
		_ = os.Rename(backup, target)
		return err
	}
	_ = os.Remove(backup)
	return nil
}
