package toolapproval

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Store struct {
	root string
}

func NewStore(root string) (*Store, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, errors.New("tool approval store root is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(absolute, 0o700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, errors.New("tool approval store root must be a regular directory")
	}
	return &Store{root: absolute}, nil
}

func (s *Store) Save(evidence Evidence) error {
	if s == nil {
		return errors.New("tool approval store is required")
	}
	if err := Validate(evidence); err != nil {
		return err
	}
	runDir := filepath.Join(s.root, evidence.RunID)
	if err := os.Mkdir(runDir, 0o700); err != nil && !os.IsExist(err) {
		return err
	}
	info, err := os.Lstat(runDir)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("tool approval run directory must be a regular directory")
	}
	path := filepath.Join(runDir, evidence.ID+".json")
	data, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("save tool approval: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return err
	}
	return nil
}

func (s *Store) List(runID string) ([]Evidence, error) {
	if s == nil {
		return nil, errors.New("tool approval store is required")
	}
	if !safeToken(runID) {
		return nil, errors.New("tool approval run id is unsafe")
	}
	runDir := filepath.Join(s.root, runID)
	info, err := os.Lstat(runDir)
	if os.IsNotExist(err) {
		return []Evidence{}, nil
	}
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, errors.New("tool approval run directory must be a regular directory")
	}
	entries, err := os.ReadDir(runDir)
	if err != nil {
		return nil, err
	}
	values := make([]Evidence, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(runDir, entry.Name())
		fileInfo, err := os.Lstat(path)
		if err != nil {
			return nil, err
		}
		if fileInfo.Mode()&os.ModeSymlink != 0 || !fileInfo.Mode().IsRegular() {
			return nil, errors.New("tool approval entry must be a regular file")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var evidence Evidence
		decoder := json.NewDecoder(strings.NewReader(string(data)))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&evidence); err != nil {
			return nil, err
		}
		if err := Validate(evidence); err != nil {
			return nil, err
		}
		if evidence.RunID != runID {
			return nil, errors.New("tool approval run id does not match storage scope")
		}
		values = append(values, evidence)
	}
	sort.Slice(values, func(i, j int) bool {
		if values[i].CreatedAt.Equal(values[j].CreatedAt) {
			return values[i].ID < values[j].ID
		}
		return values[i].CreatedAt.Before(values[j].CreatedAt)
	})
	return values, nil
}
