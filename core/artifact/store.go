package artifact

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
		return nil, errors.New("artifact storage root is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve artifact storage root: %w", err)
	}
	if err := os.MkdirAll(absolute, 0o700); err != nil {
		return nil, fmt.Errorf("create artifact storage root: %w", err)
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, errors.New("artifact storage root must be a regular directory")
	}
	return &Store{root: absolute}, nil
}

func (s *Store) Save(value Artifact) error {
	if s == nil {
		return errors.New("artifact store is required")
	}
	if err := Validate(value); err != nil {
		return fmt.Errorf("validate artifact: %w", err)
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode artifact: %w", err)
	}
	data = append(data, '\n')

	s.mu.Lock()
	defer s.mu.Unlock()

	target := filepath.Join(s.root, value.ID+".json")
	temp, err := os.CreateTemp(s.root, ".artifact-*.tmp")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	cleanup := true
	defer func() {
		_ = temp.Close()
		if cleanup {
			_ = os.Remove(tempName)
		}
	}()
	if err := temp.Chmod(0o600); err != nil {
		return err
	}
	if _, err := temp.Write(data); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if _, err := os.Lstat(target); err == nil {
		return errors.New("artifact ids are immutable and cannot be overwritten")
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(tempName, target); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func (s *Store) Load(id string) (Artifact, error) {
	if s == nil {
		return Artifact{}, errors.New("artifact store is required")
	}
	if !safeID(id, "artifact-") {
		return Artifact{}, errors.New("artifact id is invalid")
	}
	path := filepath.Join(s.root, id+".json")
	info, err := os.Lstat(path)
	if err != nil {
		return Artifact{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return Artifact{}, errors.New("artifact entry must be a regular file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Artifact{}, err
	}
	var value Artifact
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return Artifact{}, fmt.Errorf("decode artifact: %w", err)
	}
	if err := Validate(value); err != nil {
		return Artifact{}, fmt.Errorf("validate persisted artifact: %w", err)
	}
	if value.ID != id {
		return Artifact{}, errors.New("persisted artifact id does not match requested id")
	}
	return value, nil
}
