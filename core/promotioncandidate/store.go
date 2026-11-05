package promotioncandidate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/promotion"
	"github.com/FernandoVillegas27/ai-engineering-os/core/staging"
)

const SchemaVersion = 1

type Record struct {
	SchemaVersion    int                         `json:"schema_version"`
	RunID            string                      `json:"run_id"`
	OriginalRoot     string                      `json:"original_root"`
	StagedWorkspace  string                      `json:"staged_workspace"`
	PreImageDigest   string                      `json:"pre_image_digest"`
	PreImageEntries  map[string]string           `json:"pre_image_entries"`
	Declared         []staging.WorkspaceChange   `json:"declared"`
	QualityPassed    bool                        `json:"quality_passed"`
	TempParent       string                      `json:"temp_parent,omitempty"`
	CreatedAt        time.Time                   `json:"created_at"`
}

func New(runID string, request promotion.Request, now time.Time) (Record, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if strings.TrimSpace(runID) == "" {
		return Record{}, errors.New("promotion candidate run id is required")
	}
	if request.PreImage.IsZero() {
		return Record{}, errors.New("promotion candidate pre-image is required")
	}
	entries := map[string]string{}
	for path, hash := range request.PreImage.Snapshot() {
		entries[path] = hex.EncodeToString(hash[:])
	}
	record := Record{
		SchemaVersion:   SchemaVersion,
		RunID:           strings.TrimSpace(runID),
		OriginalRoot:    strings.TrimSpace(request.OriginalRoot),
		StagedWorkspace: strings.TrimSpace(request.StagedWorkspace),
		PreImageDigest:  request.PreImage.Digest(),
		PreImageEntries: entries,
		Declared:        append([]staging.WorkspaceChange(nil), request.Declared...),
		QualityPassed:   request.QualityPassed,
		TempParent:      strings.TrimSpace(request.TempParent),
		CreatedAt:       now.UTC(),
	}
	if err := Validate(record); err != nil {
		return Record{}, err
	}
	return record, nil
}

func Validate(record Record) error {
	if record.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported promotion candidate schema version: %d", record.SchemaVersion)
	}
	if !safeToken(record.RunID) {
		return errors.New("promotion candidate run id is unsafe")
	}
	if record.OriginalRoot == "" || record.StagedWorkspace == "" {
		return errors.New("promotion candidate roots are required")
	}
	if !record.QualityPassed {
		return errors.New("promotion candidate requires passed quality")
	}
	if len(record.PreImageEntries) == 0 || record.PreImageDigest == "" {
		return errors.New("promotion candidate pre-image is required")
	}
	if len(record.Declared) == 0 {
		return errors.New("promotion candidate declared changes are required")
	}
	snapshot, err := decodeSnapshot(record.PreImageEntries)
	if err != nil {
		return err
	}
	if snapshot.Digest() != record.PreImageDigest {
		return errors.New("promotion candidate pre-image digest mismatch")
	}
	for _, change := range record.Declared {
		if strings.TrimSpace(change.Path) == "" {
			return errors.New("promotion candidate contains empty declared path")
		}
		switch change.Action {
		case "create", "modify", "delete":
		default:
			return fmt.Errorf("unsupported promotion action: %s", change.Action)
		}
	}
	if record.CreatedAt.IsZero() {
		return errors.New("promotion candidate created_at is required")
	}
	return nil
}

func (record Record) Request() (promotion.Request, error) {
	if err := Validate(record); err != nil {
		return promotion.Request{}, err
	}
	snapshot, err := decodeSnapshot(record.PreImageEntries)
	if err != nil {
		return promotion.Request{}, err
	}
	return promotion.Request{
		OriginalRoot:    record.OriginalRoot,
		StagedWorkspace: record.StagedWorkspace,
		PreImage:        staging.NewPreImage(snapshot),
		Declared:        append([]staging.WorkspaceChange(nil), record.Declared...),
		QualityPassed:   true,
		TempParent:      record.TempParent,
	}, nil
}

func decodeSnapshot(entries map[string]string) (staging.WorkspaceSnapshot, error) {
	snapshot := staging.WorkspaceSnapshot{}
	for path, encoded := range entries {
		if strings.TrimSpace(path) == "" {
			return nil, errors.New("promotion candidate pre-image contains empty path")
		}
		data, err := hex.DecodeString(encoded)
		if err != nil || len(data) != sha256.Size {
			return nil, fmt.Errorf("invalid pre-image hash for %s", path)
		}
		var hash [32]byte
		copy(hash[:], data)
		snapshot[path] = hash
	}
	return snapshot, nil
}

type Store struct {
	root string
}

func NewStore(root string) (*Store, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, errors.New("promotion candidate store root is required")
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
		return nil, errors.New("promotion candidate store root must be a regular directory")
	}
	return &Store{root: absolute}, nil
}

func (s *Store) Save(record Record) error {
	if s == nil {
		return errors.New("promotion candidate store is required")
	}
	if err := Validate(record); err != nil {
		return err
	}
	path := filepath.Join(s.root, record.RunID+".json")
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("save promotion candidate: %w", err)
	}
	if _, err := file.Write(data); err != nil {
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

func (s *Store) Load(runID string) (Record, error) {
	if s == nil {
		return Record{}, errors.New("promotion candidate store is required")
	}
	if !safeToken(runID) {
		return Record{}, errors.New("promotion candidate run id is unsafe")
	}
	path := filepath.Join(s.root, runID+".json")
	info, err := os.Lstat(path)
	if err != nil {
		return Record{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return Record{}, errors.New("promotion candidate entry must be a regular file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Record{}, err
	}
	var record Record
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&record); err != nil {
		return Record{}, err
	}
	if err := Validate(record); err != nil {
		return Record{}, err
	}
	return record, nil
}

func (s *Store) List() ([]Record, error) {
	if s == nil {
		return nil, errors.New("promotion candidate store is required")
	}
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil, err
	}
	var records []Record
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		runID := strings.TrimSuffix(entry.Name(), ".json")
		record, err := s.Load(runID)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool { return records[i].CreatedAt.Before(records[j].CreatedAt) })
	return records, nil
}

func safeToken(value string) bool {
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
