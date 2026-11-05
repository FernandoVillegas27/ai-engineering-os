package controlplane

import (
	"bytes"
	"context"
	"encoding/base64"
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

const compactedHistoryPrefix = ".compacted-history-"

type CompactionResult struct {
	RunID           string    `json:"run_id"`
	ThroughSequence uint64    `json:"through_sequence"`
	DeletedFiles    int       `json:"deleted_files"`
	AlreadyPresent  bool      `json:"already_present"`
	PrefixDigest    string    `json:"prefix_digest"`
	ArchiveDigest   string    `json:"archive_digest"`
	CompactedAt     time.Time `json:"compacted_at"`
}

// CompactEventHistory installs a verified self-contained archive inside the run
// directory before removing individual event files covered by that archive.
// Once the marker exists, reads remain reconstructable even if cleanup is
// interrupted part-way through.
func CompactEventHistory(
	ctx context.Context,
	events *EventStore,
	snapshots *SnapshotStore,
	archives *EventArchiveStore,
	runID string,
	now time.Time,
) (CompactionResult, error) {
	if events == nil {
		return CompactionResult{}, errors.New("control-plane event store is required")
	}
	if snapshots == nil {
		return CompactionResult{}, errors.New("control-plane snapshot store is required")
	}
	if archives == nil {
		return CompactionResult{}, errors.New("control-plane event archive store is required")
	}
	if !safeToken(runID) {
		return CompactionResult{}, errors.New("run id is unsafe for control-plane compaction")
	}

	snapshot, err := snapshots.Latest(runID)
	if err != nil {
		return CompactionResult{}, err
	}
	archive, err := archives.Load(runID, snapshot.ThroughSequence)
	if err != nil {
		return CompactionResult{}, err
	}
	if err := VerifyEventArchiveSnapshot(archive, snapshot); err != nil {
		return CompactionResult{}, err
	}

	runDir := filepath.Join(events.root, runID)
	info, err := os.Lstat(runDir)
	if err != nil {
		return CompactionResult{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return CompactionResult{}, errors.New("control-plane run directory must be a regular directory")
	}

	lock, err := fslock.Acquire(ctx, filepath.Join(runDir, ".append.lock"), fslock.Options{})
	if err != nil {
		return CompactionResult{}, fmt.Errorf("acquire control-plane compaction lock: %w", err)
	}
	defer lock.Release()

	before, err := rawEventPrefixDigest(runDir, snapshot.ThroughSequence)
	if err != nil {
		return CompactionResult{}, err
	}
	if before != snapshot.PrefixDigest {
		return CompactionResult{}, errors.New("control-plane compaction prefix digest mismatch")
	}

	alreadyPresent := false
	existing, present, err := loadCompactedArchive(runDir)
	if err != nil {
		return CompactionResult{}, err
	}
	if present && existing.ThroughSequence > archive.ThroughSequence {
		return CompactionResult{}, errors.New("existing compacted history is newer than requested archive")
	}
	if present && existing.ThroughSequence == archive.ThroughSequence {
		alreadyPresent = true
		if err := VerifyEventArchiveSnapshot(existing, snapshot); err != nil {
			return CompactionResult{}, fmt.Errorf("existing compacted history does not match snapshot: %w", err)
		}
		if existing.RecordDigest != archive.RecordDigest {
			return CompactionResult{}, errors.New("existing compacted history does not match external archive")
		}
	} else {
		if present {
			if err := verifyArchiveExtension(existing, archive); err != nil {
				return CompactionResult{}, err
			}
		}
		if err := writeCompactedArchive(runDir, archive); err != nil {
			return CompactionResult{}, err
		}
		installed, ok, err := loadCompactedArchive(runDir)
		if err != nil {
			return CompactionResult{}, err
		}
		if !ok || installed.ThroughSequence != archive.ThroughSequence {
			return CompactionResult{}, errors.New("control-plane compacted history was not persisted")
		}
		if installed.RecordDigest != archive.RecordDigest {
			return CompactionResult{}, errors.New("control-plane compacted history digest mismatch after write")
		}
	}

	afterInstall, err := rawEventPrefixDigest(runDir, snapshot.ThroughSequence)
	if err != nil {
		return CompactionResult{}, err
	}
	if afterInstall != snapshot.PrefixDigest {
		return CompactionResult{}, errors.New("control-plane compacted history cannot reproduce snapshot prefix")
	}

	deleted, err := deleteArchivedPrefixFiles(runDir, archive)
	if err != nil {
		return CompactionResult{}, err
	}

	afterDelete, err := rawEventPrefixDigest(runDir, snapshot.ThroughSequence)
	if err != nil {
		return CompactionResult{}, err
	}
	if afterDelete != snapshot.PrefixDigest {
		return CompactionResult{}, errors.New("control-plane compacted history prefix changed after cleanup")
	}
	history, err := loadEvents(runDir)
	if err != nil {
		return CompactionResult{}, err
	}
	if uint64(len(history)) < archive.ThroughSequence {
		return CompactionResult{}, errors.New("control-plane compacted history is incomplete after cleanup")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return CompactionResult{
		RunID:           runID,
		ThroughSequence: archive.ThroughSequence,
		DeletedFiles:    deleted,
		AlreadyPresent:  alreadyPresent,
		PrefixDigest:    archive.PrefixDigest,
		ArchiveDigest:   archive.RecordDigest,
		CompactedAt:     now.UTC(),
	}, nil
}

func loadCompactedArchive(runDir string) (EventArchive, bool, error) {
	entries, err := os.ReadDir(runDir)
	if err != nil {
		return EventArchive{}, false, err
	}
	var latest EventArchive
	found := false
	for _, entry := range entries {
		name := entry.Name()
		if !isCompactedHistoryFilename(name) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return EventArchive{}, false, err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return EventArchive{}, false, fmt.Errorf("control-plane compacted history must be a regular file: %s", name)
		}
		data, err := os.ReadFile(filepath.Join(runDir, name))
		if err != nil {
			return EventArchive{}, false, err
		}
		var record EventArchive
		decoder := json.NewDecoder(strings.NewReader(string(data)))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&record); err != nil {
			return EventArchive{}, false, fmt.Errorf("decode control-plane compacted history %s: %w", name, err)
		}
		if err := ValidateEventArchive(record); err != nil {
			return EventArchive{}, false, err
		}
		expected := compactedHistoryFilename(record.ThroughSequence)
		if name != expected {
			return EventArchive{}, false, fmt.Errorf("control-plane compacted history filename mismatch: %s", name)
		}
		if !found || record.ThroughSequence > latest.ThroughSequence {
			latest = record
			found = true
		}
	}
	return latest, found, nil
}

func writeCompactedArchive(runDir string, record EventArchive) error {
	if err := ValidateEventArchive(record); err != nil {
		return err
	}
	path := filepath.Join(runDir, compactedHistoryFilename(record.ThroughSequence))
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("persist control-plane compacted history: %w", err)
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

func compactedHistoryFilename(through uint64) string {
	return fmt.Sprintf("%s%020d.json", compactedHistoryPrefix, through)
}

func isCompactedHistoryFilename(name string) bool {
	if !strings.HasPrefix(name, compactedHistoryPrefix) || !strings.HasSuffix(name, ".json") {
		return false
	}
	value := strings.TrimSuffix(strings.TrimPrefix(name, compactedHistoryPrefix), ".json")
	if len(value) != 20 {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func verifyArchiveExtension(previous, next EventArchive) error {
	if err := ValidateEventArchive(previous); err != nil {
		return err
	}
	if err := ValidateEventArchive(next); err != nil {
		return err
	}
	if previous.RunID != next.RunID || previous.ThroughSequence >= next.ThroughSequence {
		return errors.New("control-plane compacted history extension identity is invalid")
	}
	if len(previous.Entries) > len(next.Entries) {
		return errors.New("control-plane compacted history extension is shorter than previous history")
	}
	for i, entry := range previous.Entries {
		candidate := next.Entries[i]
		if entry.Sequence != candidate.Sequence ||
			entry.Filename != candidate.Filename ||
			entry.Content != candidate.Content ||
			entry.SHA256 != candidate.SHA256 {
			return fmt.Errorf("control-plane compacted history extension changed archived event %d", entry.Sequence)
		}
	}
	return nil
}

func deleteArchivedPrefixFiles(runDir string, record EventArchive) (int, error) {
	deleted := 0
	for _, archived := range record.Entries {
		path := filepath.Join(runDir, archived.Filename)
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return deleted, err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return deleted, fmt.Errorf("control-plane event entry is not regular: %s", archived.Filename)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return deleted, err
		}
		if err := verifyLiveEventMatchesArchive(record, archived.Sequence, archived.Filename, raw); err != nil {
			return deleted, err
		}
		if err := os.Remove(path); err != nil {
			return deleted, err
		}
		deleted++
	}
	return deleted, nil
}

func verifyLiveEventMatchesArchive(
	record EventArchive,
	sequence uint64,
	filename string,
	raw []byte,
) error {
	if sequence == 0 || sequence > uint64(len(record.Entries)) {
		return fmt.Errorf("control-plane live event is outside compacted archive: %d", sequence)
	}
	entry := record.Entries[sequence-1]
	if entry.Sequence != sequence || entry.Filename != filename {
		return fmt.Errorf("control-plane live event does not match compacted archive identity: %d", sequence)
	}
	expected, err := base64.StdEncoding.DecodeString(entry.Content)
	if err != nil {
		return err
	}
	if !bytes.Equal(expected, raw) {
		return fmt.Errorf("control-plane live event bytes do not match compacted archive: %d", sequence)
	}
	return nil
}

type rawHistoricalEvent struct {
	sequence uint64
	name     string
	data     []byte
}

func rawHistoricalEventPrefix(runDir string, through uint64) ([]rawHistoricalEvent, error) {
	if through == 0 {
		return nil, errors.New("event prefix sequence is required")
	}
	archive, compacted, err := loadCompactedArchive(runDir)
	if err != nil {
		return nil, err
	}

	values := make([]rawHistoricalEvent, 0, through)
	archivedThrough := uint64(0)
	if compacted {
		archivedThrough = archive.ThroughSequence
		limit := archivedThrough
		if through < limit {
			limit = through
		}
		for i := uint64(0); i < limit; i++ {
			entry := archive.Entries[i]
			raw, err := base64.StdEncoding.DecodeString(entry.Content)
			if err != nil {
				return nil, err
			}
			values = append(values, rawHistoricalEvent{
				sequence: entry.Sequence,
				name:     entry.Filename,
				data:     raw,
			})
		}
	}

	entries, err := os.ReadDir(runDir)
	if err != nil {
		return nil, err
	}
	type candidate struct {
		sequence uint64
		name     string
	}
	var live []candidate
	seen := map[uint64]bool{}
	for _, entry := range entries {
		name := entry.Name()
		if name == ".append.lock" || isCompactedHistoryFilename(name) || entry.IsDir() || filepath.Ext(name) != ".json" {
			continue
		}
		sequence := filenameSequence(name)
		if sequence == 0 {
			return nil, fmt.Errorf("control-plane event filename has invalid sequence: %s", name)
		}
		if sequence <= archivedThrough {
			raw, err := os.ReadFile(filepath.Join(runDir, name))
			if err != nil {
				return nil, err
			}
			if err := verifyLiveEventMatchesArchive(archive, sequence, name, raw); err != nil {
				return nil, err
			}
			continue
		}
		if sequence > through {
			continue
		}
		if seen[sequence] {
			return nil, fmt.Errorf("control-plane event sequence duplicate in prefix: %d", sequence)
		}
		seen[sequence] = true
		live = append(live, candidate{sequence: sequence, name: name})
	}
	sort.Slice(live, func(i, j int) bool { return live[i].sequence < live[j].sequence })
	for _, candidate := range live {
		path := filepath.Join(runDir, candidate.name)
		info, err := os.Lstat(path)
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return nil, fmt.Errorf("control-plane event entry is not regular: %s", candidate.name)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		values = append(values, rawHistoricalEvent{
			sequence: candidate.sequence,
			name:     candidate.name,
			data:     raw,
		})
	}

	sort.Slice(values, func(i, j int) bool { return values[i].sequence < values[j].sequence })
	if uint64(len(values)) != through {
		return nil, fmt.Errorf(
			"control-plane event prefix is incomplete: got %d events want %d",
			len(values),
			through,
		)
	}
	for i, value := range values {
		expected := uint64(i + 1)
		if value.sequence != expected {
			return nil, fmt.Errorf(
				"control-plane event prefix sequence mismatch: got %d want %d",
				value.sequence,
				expected,
			)
		}
	}
	return values, nil
}
