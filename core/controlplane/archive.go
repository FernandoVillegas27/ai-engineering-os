package controlplane

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/fslock"
)

const EventArchiveSchemaVersion = 1

type ArchivedEvent struct {
	Sequence uint64 `json:"sequence"`
	Filename string `json:"filename"`
	Content  string `json:"content_base64"`
	SHA256   string `json:"sha256"`
}

type EventArchive struct {
	SchemaVersion        int             `json:"schema_version"`
	RunID                string          `json:"run_id"`
	ThroughSequence      uint64          `json:"through_sequence"`
	PrefixDigest         string          `json:"prefix_digest"`
	SnapshotRecordDigest string          `json:"snapshot_record_digest"`
	Entries              []ArchivedEvent `json:"entries"`
	CreatedAt            time.Time       `json:"created_at"`
	RecordDigest         string          `json:"record_digest"`
}

type EventArchiveStore struct {
	root string
}

func NewEventArchiveStore(root string) (*EventArchiveStore, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, errors.New("control-plane event archive storage root is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve control-plane event archive storage root: %w", err)
	}
	if err := os.MkdirAll(absolute, 0o700); err != nil {
		return nil, fmt.Errorf("create control-plane event archive storage root: %w", err)
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, errors.New("control-plane event archive storage root must be a regular directory")
	}
	return &EventArchiveStore{root: absolute}, nil
}

// Create persists an immutable copy of the exact raw event prefix covered by
// the latest verified snapshot. It deliberately does not delete live events.
func (s *EventArchiveStore) Create(
	ctx context.Context,
	events *EventStore,
	snapshots *SnapshotStore,
	runID string,
	now time.Time,
) (EventArchive, error) {
	if s == nil {
		return EventArchive{}, errors.New("control-plane event archive store is required")
	}
	if events == nil {
		return EventArchive{}, errors.New("control-plane event store is required")
	}
	if snapshots == nil {
		return EventArchive{}, errors.New("control-plane snapshot store is required")
	}
	if !safeToken(runID) {
		return EventArchive{}, errors.New("run id is unsafe for control-plane event archive storage")
	}

	snapshot, err := snapshots.Latest(runID)
	if err != nil {
		return EventArchive{}, err
	}
	if err := ValidateSnapshot(snapshot); err != nil {
		return EventArchive{}, err
	}

	runDir := filepath.Join(events.root, runID)
	info, err := os.Lstat(runDir)
	if err != nil {
		return EventArchive{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return EventArchive{}, errors.New("control-plane run directory must be a regular directory")
	}

	lock, err := fslock.Acquire(ctx, filepath.Join(runDir, ".append.lock"), fslock.Options{})
	if err != nil {
		return EventArchive{}, fmt.Errorf("acquire control-plane event archive lock: %w", err)
	}
	defer lock.Release()

	prefixDigest, err := rawEventPrefixDigest(runDir, snapshot.ThroughSequence)
	if err != nil {
		return EventArchive{}, err
	}
	if prefixDigest != snapshot.PrefixDigest {
		return EventArchive{}, errors.New("control-plane snapshot event prefix digest mismatch")
	}

	entries, err := collectArchivedEventPrefix(runDir, snapshot.RunID, snapshot.ThroughSequence)
	if err != nil {
		return EventArchive{}, err
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	record := EventArchive{
		SchemaVersion:        EventArchiveSchemaVersion,
		RunID:                snapshot.RunID,
		ThroughSequence:      snapshot.ThroughSequence,
		PrefixDigest:         snapshot.PrefixDigest,
		SnapshotRecordDigest: snapshot.RecordDigest,
		Entries:              entries,
		CreatedAt:            now.UTC(),
	}
	record.RecordDigest, err = eventArchiveRecordDigest(record)
	if err != nil {
		return EventArchive{}, err
	}
	if err := ValidateEventArchive(record); err != nil {
		return EventArchive{}, err
	}
	if err := s.save(record); err != nil {
		return EventArchive{}, err
	}
	return record, nil
}

func (s *EventArchiveStore) Load(runID string, through uint64) (EventArchive, error) {
	if s == nil {
		return EventArchive{}, errors.New("control-plane event archive store is required")
	}
	if !safeToken(runID) || through == 0 {
		return EventArchive{}, errors.New("control-plane event archive identity is invalid")
	}
	path := s.archivePath(runID, through)
	info, err := os.Lstat(path)
	if err != nil {
		return EventArchive{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return EventArchive{}, errors.New("control-plane event archive entry must be a regular file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return EventArchive{}, err
	}
	var record EventArchive
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&record); err != nil {
		return EventArchive{}, fmt.Errorf("decode control-plane event archive: %w", err)
	}
	if record.RunID != runID || record.ThroughSequence != through {
		return EventArchive{}, errors.New("control-plane event archive identity mismatch")
	}
	if err := ValidateEventArchive(record); err != nil {
		return EventArchive{}, err
	}
	return record, nil
}

func (s *EventArchiveStore) Latest(runID string) (EventArchive, error) {
	if s == nil {
		return EventArchive{}, errors.New("control-plane event archive store is required")
	}
	if !safeToken(runID) {
		return EventArchive{}, errors.New("run id is unsafe for control-plane event archive storage")
	}
	runDir := filepath.Join(s.root, runID)
	info, err := os.Lstat(runDir)
	if err != nil {
		return EventArchive{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return EventArchive{}, errors.New("control-plane event archive run directory must be regular")
	}
	entries, err := os.ReadDir(runDir)
	if err != nil {
		return EventArchive{}, err
	}
	var latest uint64
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return EventArchive{}, err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return EventArchive{}, fmt.Errorf("control-plane event archive entry is not regular: %s", entry.Name())
		}
		sequence, ok := eventArchiveFilenameSequence(entry.Name())
		if !ok {
			return EventArchive{}, fmt.Errorf("unexpected control-plane event archive entry: %s", entry.Name())
		}
		if sequence > latest {
			latest = sequence
		}
	}
	if latest == 0 {
		return EventArchive{}, os.ErrNotExist
	}
	return s.Load(runID, latest)
}

func ValidateEventArchive(record EventArchive) error {
	if record.SchemaVersion != EventArchiveSchemaVersion {
		return fmt.Errorf("unsupported control-plane event archive schema version: %d", record.SchemaVersion)
	}
	if !safeToken(record.RunID) {
		return errors.New("control-plane event archive run id is invalid")
	}
	if record.ThroughSequence == 0 {
		return errors.New("control-plane event archive through_sequence is required")
	}
	if !validSHA256(record.PrefixDigest) {
		return errors.New("control-plane event archive prefix digest is invalid")
	}
	if !validSHA256(record.SnapshotRecordDigest) {
		return errors.New("control-plane event archive snapshot record digest is invalid")
	}
	if record.CreatedAt.IsZero() {
		return errors.New("control-plane event archive created_at is required")
	}
	if uint64(len(record.Entries)) != record.ThroughSequence {
		return fmt.Errorf(
			"control-plane event archive entry count mismatch: got %d want %d",
			len(record.Entries),
			record.ThroughSequence,
		)
	}

	digest := sha256.New()
	for i, entry := range record.Entries {
		expected := uint64(i + 1)
		if entry.Sequence != expected {
			return fmt.Errorf(
				"control-plane event archive sequence mismatch: got %d want %d",
				entry.Sequence,
				expected,
			)
		}
		if filenameSequence(entry.Filename) != entry.Sequence {
			return fmt.Errorf("control-plane event archive filename sequence mismatch: %s", entry.Filename)
		}
		raw, err := base64.StdEncoding.DecodeString(entry.Content)
		if err != nil {
			return fmt.Errorf("decode archived control-plane event %d: %w", entry.Sequence, err)
		}
		sum := sha256.Sum256(raw)
		if hex.EncodeToString(sum[:]) != entry.SHA256 {
			return fmt.Errorf("archived control-plane event digest mismatch: %d", entry.Sequence)
		}
		var value Event
		decoder := json.NewDecoder(strings.NewReader(string(raw)))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&value); err != nil {
			return fmt.Errorf("decode archived control-plane event %d: %w", entry.Sequence, err)
		}
		if err := ValidateEvent(value); err != nil {
			return fmt.Errorf("validate archived control-plane event %d: %w", entry.Sequence, err)
		}
		if value.RunID != record.RunID || value.Sequence != entry.Sequence {
			return fmt.Errorf("archived control-plane event identity mismatch: %d", entry.Sequence)
		}
		writeDigestPart(digest, entry.Filename)
		writeDigestPart(digest, string(raw))
	}
	if hex.EncodeToString(digest.Sum(nil)) != record.PrefixDigest {
		return errors.New("control-plane event archive prefix digest mismatch")
	}
	if !validSHA256(record.RecordDigest) {
		return errors.New("control-plane event archive record digest is invalid")
	}
	expected, err := eventArchiveRecordDigest(record)
	if err != nil {
		return err
	}
	if expected != record.RecordDigest {
		return errors.New("control-plane event archive record digest mismatch")
	}
	return nil
}

func VerifyEventArchiveSnapshot(record EventArchive, snapshot Snapshot) error {
	if err := ValidateEventArchive(record); err != nil {
		return err
	}
	if err := ValidateSnapshot(snapshot); err != nil {
		return err
	}
	if record.RunID != snapshot.RunID ||
		record.ThroughSequence != snapshot.ThroughSequence ||
		record.PrefixDigest != snapshot.PrefixDigest ||
		record.SnapshotRecordDigest != snapshot.RecordDigest {
		return errors.New("control-plane event archive does not match snapshot")
	}
	return nil
}

func EventsFromArchive(record EventArchive) ([]Event, error) {
	if err := ValidateEventArchive(record); err != nil {
		return nil, err
	}
	values := make([]Event, 0, len(record.Entries))
	for _, entry := range record.Entries {
		raw, err := base64.StdEncoding.DecodeString(entry.Content)
		if err != nil {
			return nil, err
		}
		var value Event
		decoder := json.NewDecoder(strings.NewReader(string(raw)))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}

func collectArchivedEventPrefix(runDir, runID string, through uint64) ([]ArchivedEvent, error) {
	entries, err := os.ReadDir(runDir)
	if err != nil {
		return nil, err
	}
	type candidate struct {
		sequence uint64
		name     string
	}
	var values []candidate
	seen := map[uint64]bool{}
	for _, entry := range entries {
		name := entry.Name()
		if name == ".append.lock" || entry.IsDir() || filepath.Ext(name) != ".json" {
			continue
		}
		sequence := filenameSequence(name)
		if sequence == 0 {
			return nil, fmt.Errorf("control-plane event filename has invalid sequence: %s", name)
		}
		if sequence > through {
			continue
		}
		if seen[sequence] {
			return nil, fmt.Errorf("control-plane event sequence duplicate in prefix: %d", sequence)
		}
		seen[sequence] = true
		values = append(values, candidate{sequence: sequence, name: name})
	}
	sort.Slice(values, func(i, j int) bool { return values[i].sequence < values[j].sequence })
	if uint64(len(values)) != through {
		return nil, fmt.Errorf(
			"control-plane event prefix is incomplete: got %d events want %d",
			len(values),
			through,
		)
	}

	archived := make([]ArchivedEvent, 0, len(values))
	for i, value := range values {
		expected := uint64(i + 1)
		if value.sequence != expected {
			return nil, fmt.Errorf(
				"control-plane event prefix sequence mismatch: got %d want %d",
				value.sequence,
				expected,
			)
		}
		path := filepath.Join(runDir, value.name)
		info, err := os.Lstat(path)
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return nil, fmt.Errorf("control-plane event entry is not regular: %s", value.name)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var event Event
		decoder := json.NewDecoder(strings.NewReader(string(raw)))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&event); err != nil {
			return nil, fmt.Errorf("decode control-plane event %s: %w", value.name, err)
		}
		if err := ValidateEvent(event); err != nil {
			return nil, fmt.Errorf("validate control-plane event %s: %w", value.name, err)
		}
		if event.RunID != runID || event.Sequence != value.sequence {
			return nil, fmt.Errorf("control-plane event identity mismatch: %s", value.name)
		}
		sum := sha256.Sum256(raw)
		archived = append(archived, ArchivedEvent{
			Sequence: value.sequence,
			Filename: value.name,
			Content:  base64.StdEncoding.EncodeToString(raw),
			SHA256:   hex.EncodeToString(sum[:]),
		})
	}
	return archived, nil
}

func eventArchiveRecordDigest(record EventArchive) (string, error) {
	copy := record
	copy.RecordDigest = ""
	data, err := json.Marshal(copy)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func (s *EventArchiveStore) save(record EventArchive) error {
	runDir := filepath.Join(s.root, record.RunID)
	if err := os.Mkdir(runDir, 0o700); err != nil && !os.IsExist(err) {
		return err
	}
	info, err := os.Lstat(runDir)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("control-plane event archive run directory must be regular")
	}
	path := s.archivePath(record.RunID, record.ThroughSequence)
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return fmt.Errorf(
				"control-plane event archive already exists for %s through sequence %d",
				record.RunID,
				record.ThroughSequence,
			)
		}
		return err
	}
	cleanup := true
	defer func() {
		_ = file.Close()
		if cleanup {
			_ = os.Remove(path)
		}
	}()
	if _, err := file.Write(data); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func (s *EventArchiveStore) archivePath(runID string, through uint64) string {
	return filepath.Join(s.root, runID, fmt.Sprintf("%020d.archive.json", through))
}

func eventArchiveFilenameSequence(name string) (uint64, bool) {
	if !strings.HasSuffix(name, ".archive.json") {
		return 0, false
	}
	prefix := strings.TrimSuffix(name, ".archive.json")
	if len(prefix) != 20 {
		return 0, false
	}
	var value uint64
	for _, r := range prefix {
		if r < '0' || r > '9' {
			return 0, false
		}
		value = value*10 + uint64(r-'0')
	}
	return value, value > 0
}
