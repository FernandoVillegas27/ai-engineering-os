package controlplane

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/fslock"
)

const SnapshotSchemaVersion = 1

type Snapshot struct {
	SchemaVersion   int       `json:"schema_version"`
	RunID           string    `json:"run_id"`
	ThroughSequence uint64    `json:"through_sequence"`
	PrefixDigest    string    `json:"prefix_digest"`
	State           State     `json:"state"`
	CreatedAt       time.Time `json:"created_at"`
	RecordDigest    string    `json:"record_digest"`
}

type SnapshotStore struct {
	root string
}

func NewSnapshotStore(root string) (*SnapshotStore, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, errors.New("control-plane snapshot storage root is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve control-plane snapshot storage root: %w", err)
	}
	if err := os.MkdirAll(absolute, 0o700); err != nil {
		return nil, fmt.Errorf("create control-plane snapshot storage root: %w", err)
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, errors.New("control-plane snapshot storage root must be a regular directory")
	}
	return &SnapshotStore{root: absolute}, nil
}

// Create builds an immutable snapshot only after the complete event prefix has
// passed normal event loading and projection validation.
func (s *SnapshotStore) Create(
	ctx context.Context,
	events *EventStore,
	runID string,
	now time.Time,
) (Snapshot, error) {
	if s == nil {
		return Snapshot{}, errors.New("control-plane snapshot store is required")
	}
	if events == nil {
		return Snapshot{}, errors.New("control-plane event store is required")
	}
	if !safeToken(runID) {
		return Snapshot{}, errors.New("run id is unsafe for control-plane snapshot storage")
	}
	runDir := filepath.Join(events.root, runID)
	info, err := os.Lstat(runDir)
	if err != nil {
		return Snapshot{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return Snapshot{}, errors.New("control-plane run directory must be a regular directory")
	}

	lock, err := fslock.Acquire(ctx, filepath.Join(runDir, ".append.lock"), fslock.Options{})
	if err != nil {
		return Snapshot{}, fmt.Errorf("acquire control-plane snapshot lock: %w", err)
	}
	defer lock.Release()

	values, err := loadEvents(runDir)
	if err != nil {
		return Snapshot{}, err
	}
	if len(values) == 0 {
		return Snapshot{}, errors.New("cannot snapshot an empty control-plane run")
	}
	state, err := Project(values)
	if err != nil {
		return Snapshot{}, err
	}
	prefixDigest, err := rawEventPrefixDigest(runDir, state.LastSequence)
	if err != nil {
		return Snapshot{}, err
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	record := Snapshot{
		SchemaVersion:   SnapshotSchemaVersion,
		RunID:           runID,
		ThroughSequence: state.LastSequence,
		PrefixDigest:    prefixDigest,
		State:           state,
		CreatedAt:       now.UTC(),
	}
	digest, err := snapshotRecordDigest(record)
	if err != nil {
		return Snapshot{}, err
	}
	record.RecordDigest = digest
	if err := ValidateSnapshot(record); err != nil {
		return Snapshot{}, err
	}
	if err := s.save(record); err != nil {
		return Snapshot{}, err
	}
	return record, nil
}

func (s *SnapshotStore) Load(runID string, through uint64) (Snapshot, error) {
	if s == nil {
		return Snapshot{}, errors.New("control-plane snapshot store is required")
	}
	if !safeToken(runID) || through == 0 {
		return Snapshot{}, errors.New("control-plane snapshot identity is invalid")
	}
	path := s.snapshotPath(runID, through)
	info, err := os.Lstat(path)
	if err != nil {
		return Snapshot{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return Snapshot{}, errors.New("control-plane snapshot entry must be a regular file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Snapshot{}, err
	}
	var record Snapshot
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&record); err != nil {
		return Snapshot{}, fmt.Errorf("decode control-plane snapshot: %w", err)
	}
	if record.RunID != runID || record.ThroughSequence != through {
		return Snapshot{}, errors.New("control-plane snapshot identity mismatch")
	}
	if err := ValidateSnapshot(record); err != nil {
		return Snapshot{}, err
	}
	return record, nil
}

func (s *SnapshotStore) Latest(runID string) (Snapshot, error) {
	if s == nil {
		return Snapshot{}, errors.New("control-plane snapshot store is required")
	}
	if !safeToken(runID) {
		return Snapshot{}, errors.New("run id is unsafe for control-plane snapshot storage")
	}
	runDir := filepath.Join(s.root, runID)
	info, err := os.Lstat(runDir)
	if err != nil {
		return Snapshot{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return Snapshot{}, errors.New("control-plane snapshot run directory must be regular")
	}
	entries, err := os.ReadDir(runDir)
	if err != nil {
		return Snapshot{}, err
	}
	var latest uint64
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return Snapshot{}, err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return Snapshot{}, fmt.Errorf("control-plane snapshot entry is not regular: %s", entry.Name())
		}
		sequence, ok := snapshotFilenameSequence(entry.Name())
		if !ok {
			return Snapshot{}, fmt.Errorf("unexpected control-plane snapshot entry: %s", entry.Name())
		}
		if sequence > latest {
			latest = sequence
		}
	}
	if latest == 0 {
		return Snapshot{}, os.ErrNotExist
	}
	return s.Load(runID, latest)
}

// Verify proves that the immutable event bytes covered by the snapshot still
// match the prefix that was fully validated when the snapshot was created.
func (s *SnapshotStore) Verify(
	ctx context.Context,
	events *EventStore,
	record Snapshot,
) error {
	if s == nil {
		return errors.New("control-plane snapshot store is required")
	}
	if events == nil {
		return errors.New("control-plane event store is required")
	}
	if err := ValidateSnapshot(record); err != nil {
		return err
	}
	runDir := filepath.Join(events.root, record.RunID)
	info, err := os.Lstat(runDir)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("control-plane run directory must be a regular directory")
	}
	lock, err := fslock.Acquire(ctx, filepath.Join(runDir, ".append.lock"), fslock.Options{})
	if err != nil {
		return fmt.Errorf("acquire control-plane snapshot verification lock: %w", err)
	}
	defer lock.Release()

	digest, err := rawEventPrefixDigest(runDir, record.ThroughSequence)
	if err != nil {
		return err
	}
	if digest != record.PrefixDigest {
		return errors.New("control-plane snapshot event prefix digest mismatch")
	}
	return nil
}

func ValidateSnapshot(record Snapshot) error {
	if record.SchemaVersion != SnapshotSchemaVersion {
		return fmt.Errorf("unsupported control-plane snapshot schema version: %d", record.SchemaVersion)
	}
	if !safeToken(record.RunID) {
		return errors.New("control-plane snapshot run id is invalid")
	}
	if record.ThroughSequence == 0 {
		return errors.New("control-plane snapshot through_sequence is required")
	}
	if !validSHA256(record.PrefixDigest) {
		return errors.New("control-plane snapshot prefix digest is invalid")
	}
	if record.State.RunID != record.RunID {
		return errors.New("control-plane snapshot state run id mismatch")
	}
	if record.State.LastSequence != record.ThroughSequence {
		return errors.New("control-plane snapshot state sequence mismatch")
	}
	if record.CreatedAt.IsZero() {
		return errors.New("control-plane snapshot created_at is required")
	}
	if !validSHA256(record.RecordDigest) {
		return errors.New("control-plane snapshot record digest is invalid")
	}
	expected, err := snapshotRecordDigest(record)
	if err != nil {
		return err
	}
	if expected != record.RecordDigest {
		return errors.New("control-plane snapshot record digest mismatch")
	}
	if err := validateSnapshotDerivedState(record.State); err != nil {
		return err
	}
	return nil
}

func (s *SnapshotStore) save(record Snapshot) error {
	runDir := filepath.Join(s.root, record.RunID)
	if err := os.Mkdir(runDir, 0o700); err != nil && !os.IsExist(err) {
		return err
	}
	info, err := os.Lstat(runDir)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("control-plane snapshot run directory must be regular")
	}
	path := s.snapshotPath(record.RunID, record.ThroughSequence)
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return fmt.Errorf(
				"control-plane snapshot already exists for %s through sequence %d",
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

func (s *SnapshotStore) snapshotPath(runID string, through uint64) string {
	return filepath.Join(
		s.root,
		runID,
		fmt.Sprintf("%020d.snapshot.json", through),
	)
}

func snapshotFilenameSequence(name string) (uint64, bool) {
	if !strings.HasSuffix(name, ".snapshot.json") {
		return 0, false
	}
	prefix := strings.TrimSuffix(name, ".snapshot.json")
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

func snapshotRecordDigest(record Snapshot) (string, error) {
	copy := record
	copy.RecordDigest = ""
	data, err := json.Marshal(copy)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func validateSnapshotDerivedState(state State) error {
	clone := state
	originalProgress := clone.Progress
	originalAgents := clone.Agents
	originalEdges := clone.Edges
	clone.recompute()
	if !reflect.DeepEqual(originalProgress, clone.Progress) {
		return errors.New("control-plane snapshot progress is inconsistent with tasks")
	}
	if !reflect.DeepEqual(originalAgents, clone.Agents) {
		return errors.New("control-plane snapshot agents are inconsistent with tasks")
	}
	if !reflect.DeepEqual(originalEdges, clone.Edges) {
		return errors.New("control-plane snapshot edges are inconsistent with tasks")
	}
	return nil
}

func rawEventPrefixDigest(runDir string, through uint64) (string, error) {
	values, err := rawHistoricalEventPrefix(runDir, through)
	if err != nil {
		return "", err
	}
	digest := sha256.New()
	for _, value := range values {
		writeDigestPart(digest, value.name)
		writeDigestPart(digest, string(value.data))
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func writeDigestPart(target hash.Hash, value string) {
	_, _ = target.Write([]byte(value))
	_, _ = target.Write([]byte{0})
}

func validSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
