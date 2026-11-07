package memory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/fslock"
)

const GovernanceSchemaVersion = 1

type GovernanceAction string

const (
	GovernancePending    GovernanceAction = "pending"
	GovernanceSaved      GovernanceAction = "saved"
	GovernanceSuperseded GovernanceAction = "superseded"
	GovernanceReviewed   GovernanceAction = "reviewed"
	GovernanceDeleted    GovernanceAction = "deleted"
)

type GovernanceEntry struct {
	SchemaVersion int              `json:"schema_version"`
	Sequence      uint64           `json:"sequence"`
	Action        GovernanceAction `json:"action"`
	Project       string           `json:"project"`
	GovernanceID  string           `json:"governance_id,omitempty"`
	ReferenceID   string           `json:"reference_id,omitempty"`
	Supersedes    string           `json:"supersedes,omitempty"`
	Provider      string           `json:"provider,omitempty"`
	MemoryID      string           `json:"memory_id,omitempty"`
	TopicKey      string           `json:"topic_key,omitempty"`
	Title         string           `json:"title,omitempty"`
	Content       string           `json:"content,omitempty"`
	ContentSHA256 string           `json:"content_sha256,omitempty"`
	Type          Type             `json:"type,omitempty"`
	Provenance    Provenance       `json:"provenance,omitempty"`
	HardDelete    bool             `json:"hard_delete,omitempty"`
	CreatedAt     time.Time        `json:"created_at"`
}

type GovernanceState string

const (
	GovernancePendingState    GovernanceState = "pending"
	GovernanceActive          GovernanceState = "active"
	GovernanceSupersededState GovernanceState = "superseded"
	GovernanceDeletedState    GovernanceState = "deleted"
)

type GovernedRecord struct {
	GovernanceID  string          `json:"governance_id"`
	Project       string          `json:"project"`
	Provider      string          `json:"provider"`
	MemoryID      string          `json:"memory_id,omitempty"`
	TopicKey      string          `json:"topic_key,omitempty"`
	Title         string          `json:"title"`
	Content       string          `json:"content"`
	ContentSHA256 string          `json:"content_sha256"`
	Type          Type            `json:"type"`
	Provenance    Provenance      `json:"provenance,omitempty"`
	State         GovernanceState `json:"state"`
	Supersedes    string          `json:"supersedes,omitempty"`
	SupersededBy  string          `json:"superseded_by,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	ReviewedAt    *time.Time      `json:"reviewed_at,omitempty"`
	DeletedAt     *time.Time      `json:"deleted_at,omitempty"`
	HardDeleted   bool            `json:"hard_deleted,omitempty"`
}

type GovernanceStore struct {
	root string
	now  func() time.Time
}

func NewGovernanceStore(root string) (*GovernanceStore, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, errors.New("memory governance root is required")
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
		return nil, errors.New("memory governance root must be a regular directory")
	}
	return &GovernanceStore{root: absolute, now: func() time.Time { return time.Now().UTC() }}, nil
}

func (s *GovernanceStore) Append(ctx context.Context, entry GovernanceEntry) (GovernanceEntry, error) {
	if s == nil {
		return GovernanceEntry{}, errors.New("memory governance store is required")
	}
	if err := validateProject(entry.Project); err != nil {
		return GovernanceEntry{}, err
	}
	if !validGovernanceAction(entry.Action) {
		return GovernanceEntry{}, fmt.Errorf("unsupported memory governance action: %q", entry.Action)
	}
	projectDir := filepath.Join(s.root, governanceProjectKey(entry.Project))
	if err := os.MkdirAll(projectDir, 0o700); err != nil {
		return GovernanceEntry{}, err
	}
	info, err := os.Lstat(projectDir)
	if err != nil {
		return GovernanceEntry{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return GovernanceEntry{}, errors.New("memory governance project directory must be regular")
	}

	lock, err := fslock.Acquire(ctx, filepath.Join(projectDir, ".ledger.lock"), fslock.Options{})
	if err != nil {
		return GovernanceEntry{}, err
	}
	defer lock.Release()

	existing, err := s.listUnlocked(projectDir, entry.Project)
	if err != nil {
		return GovernanceEntry{}, err
	}
	entry.SchemaVersion = GovernanceSchemaVersion
	entry.Sequence = uint64(len(existing) + 1)
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = s.now().UTC()
	} else {
		entry.CreatedAt = entry.CreatedAt.UTC()
	}
	if entry.Action == GovernancePending {
		if strings.TrimSpace(entry.Title) == "" || strings.TrimSpace(entry.Content) == "" {
			return GovernanceEntry{}, errors.New("governance pending save requires title and content")
		}
		sum := sha256.Sum256([]byte(entry.Content))
		entry.ContentSHA256 = hex.EncodeToString(sum[:])
		if entry.GovernanceID == "" {
			entry.GovernanceID = governanceRecordID(entry.Project, entry.TopicKey, entry.Title, entry.Content, entry.Sequence)
		}
	}
	if entry.Action == GovernanceSaved || entry.Action == GovernanceSuperseded ||
		entry.Action == GovernanceReviewed || entry.Action == GovernanceDeleted {
		if strings.TrimSpace(entry.ReferenceID) == "" {
			return GovernanceEntry{}, errors.New("governance mutation requires reference id")
		}
	}
	if err := validateGovernanceEntry(entry); err != nil {
		return GovernanceEntry{}, err
	}
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return GovernanceEntry{}, err
	}
	data = append(data, '\n')
	path := filepath.Join(projectDir, fmt.Sprintf("%020d.json", entry.Sequence))
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return GovernanceEntry{}, err
	}
	cleanup := true
	defer func() {
		_ = file.Close()
		if cleanup {
			_ = os.Remove(path)
		}
	}()
	if _, err := file.Write(data); err != nil {
		return GovernanceEntry{}, err
	}
	if err := file.Sync(); err != nil {
		return GovernanceEntry{}, err
	}
	if err := file.Close(); err != nil {
		return GovernanceEntry{}, err
	}
	cleanup = false
	return entry, nil
}

func (s *GovernanceStore) Entries(project string) ([]GovernanceEntry, error) {
	if s == nil {
		return nil, errors.New("memory governance store is required")
	}
	if err := validateProject(project); err != nil {
		return nil, err
	}
	projectDir := filepath.Join(s.root, governanceProjectKey(project))
	info, err := os.Lstat(projectDir)
	if os.IsNotExist(err) {
		return []GovernanceEntry{}, nil
	}
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, errors.New("memory governance project directory must be regular")
	}
	return s.listUnlocked(projectDir, project)
}

func (s *GovernanceStore) Records(project string) ([]GovernedRecord, error) {
	entries, err := s.Entries(project)
	if err != nil {
		return nil, err
	}
	return projectRecords(entries, project)
}

func (s *GovernanceStore) Record(project, governanceID string) (GovernedRecord, error) {
	records, err := s.Records(project)
	if err != nil {
		return GovernedRecord{}, err
	}
	for _, record := range records {
		if record.GovernanceID == governanceID {
			return record, nil
		}
	}
	return GovernedRecord{}, os.ErrNotExist
}

func (s *GovernanceStore) listUnlocked(projectDir, project string) ([]GovernanceEntry, error) {
	entries, err := os.ReadDir(projectDir)
	if err != nil {
		return nil, err
	}
	var out []GovernanceEntry
	for _, item := range entries {
		name := item.Name()
		if name == ".ledger.lock" || item.IsDir() {
			continue
		}
		if filepath.Ext(name) != ".json" {
			return nil, fmt.Errorf("unexpected memory governance entry: %s", name)
		}
		info, err := item.Info()
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return nil, fmt.Errorf("memory governance entry is not regular: %s", name)
		}
		data, err := os.ReadFile(filepath.Join(projectDir, name))
		if err != nil {
			return nil, err
		}
		var entry GovernanceEntry
		decoder := json.NewDecoder(strings.NewReader(string(data)))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&entry); err != nil {
			return nil, err
		}
		if err := validateGovernanceEntry(entry); err != nil {
			return nil, err
		}
		if entry.Project != project {
			return nil, errors.New("memory governance project boundary mismatch")
		}
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Sequence < out[j].Sequence })
	for i, entry := range out {
		if entry.Sequence != uint64(i+1) {
			return nil, fmt.Errorf("memory governance sequence gap: got %d want %d", entry.Sequence, i+1)
		}
	}
	return out, nil
}

func projectRecords(entries []GovernanceEntry, project string) ([]GovernedRecord, error) {
	byID := map[string]*GovernedRecord{}
	order := []string{}
	for _, entry := range entries {
		if entry.Project != project {
			return nil, errors.New("memory governance project boundary mismatch")
		}
		switch entry.Action {
		case GovernancePending:
			if _, exists := byID[entry.GovernanceID]; exists {
				return nil, fmt.Errorf("duplicate pending governance record: %s", entry.GovernanceID)
			}
			record := &GovernedRecord{
				GovernanceID: entry.GovernanceID,
				Project: entry.Project,
				TopicKey: entry.TopicKey,
				Title: entry.Title,
				Content: entry.Content,
				ContentSHA256: entry.ContentSHA256,
				Type: entry.Type,
				Provenance: entry.Provenance,
				State: GovernancePendingState,
				Supersedes: entry.Supersedes,
				CreatedAt: entry.CreatedAt,
			}
			byID[entry.GovernanceID] = record
			order = append(order, entry.GovernanceID)
		case GovernanceSaved, GovernanceSuperseded:
			record := byID[entry.ReferenceID]
			if record == nil {
				return nil, fmt.Errorf("memory governance commit references unknown pending record: %s", entry.ReferenceID)
			}
			if record.State != GovernancePendingState {
				return nil, fmt.Errorf("memory governance commit requires pending record: %s", entry.ReferenceID)
			}
			record.Provider = entry.Provider
			record.MemoryID = entry.MemoryID
			record.State = GovernanceActive
			record.Supersedes = entry.Supersedes
			if entry.Supersedes != "" {
				previous := byID[entry.Supersedes]
				if previous == nil {
					return nil, fmt.Errorf("memory governance supersedes unknown record: %s", entry.Supersedes)
				}
				if previous.State != GovernanceActive {
					return nil, fmt.Errorf("memory governance supersedes non-active record: %s", entry.Supersedes)
				}
				previous.State = GovernanceSupersededState
				previous.SupersededBy = record.GovernanceID
			}
		case GovernanceReviewed:
			record := byID[entry.ReferenceID]
			if record == nil {
				return nil, fmt.Errorf("memory governance review references unknown record: %s", entry.ReferenceID)
			}
			if record.State == GovernanceDeletedState || record.State == GovernancePendingState {
				return nil, fmt.Errorf("memory governance review requires committed record: %s", entry.ReferenceID)
			}
			at := entry.CreatedAt
			record.ReviewedAt = &at
		case GovernanceDeleted:
			record := byID[entry.ReferenceID]
			if record == nil {
				return nil, fmt.Errorf("memory governance delete references unknown record: %s", entry.ReferenceID)
			}
			if record.State != GovernanceActive {
				return nil, fmt.Errorf("memory governance delete requires active record: %s", entry.ReferenceID)
			}
			at := entry.CreatedAt
			record.State = GovernanceDeletedState
			record.DeletedAt = &at
			record.HardDeleted = entry.HardDelete
		}
	}
	out := make([]GovernedRecord, 0, len(order))
	for _, id := range order {
		out = append(out, *byID[id])
	}
	return out, nil
}

type GovernedProvider struct {
	Inner ProjectMemory
	Store *GovernanceStore
}

func (p GovernedProvider) ID() string {
	if p.Inner == nil { return "governed-memory" }
	return p.Inner.ID()
}
func (p GovernedProvider) Status(ctx context.Context) (Status, error) {
	if p.Inner == nil { return Status{}, errors.New("memory provider is required") }
	return p.Inner.Status(ctx)
}
func (p GovernedProvider) Recall(ctx context.Context, request RecallRequest) (RecallResult, error) {
	if p.Inner == nil { return RecallResult{}, errors.New("memory provider is required") }
	return p.Inner.Recall(ctx, request)
}
func (p GovernedProvider) Save(ctx context.Context, request SaveRequest) (SaveResult, error) {
	if p.Inner == nil || p.Store == nil {
		return SaveResult{}, errors.New("governed memory provider is incomplete")
	}
	redacted := RedactSaveRequest(request)
	if err := ValidateSaveRequest(redacted); err != nil {
		return SaveResult{}, err
	}
	records, err := p.Store.Records(redacted.Project)
	if err != nil {
		return SaveResult{}, err
	}
	var supersedes string
	for i := len(records)-1; i >= 0; i-- {
		record := records[i]
		if record.State == GovernanceActive &&
			strings.TrimSpace(redacted.TopicKey) != "" &&
			record.TopicKey == redacted.TopicKey {
			supersedes = record.GovernanceID
			break
		}
	}

	pending, err := p.Store.Append(ctx, GovernanceEntry{
		Action: GovernancePending,
		Project: redacted.Project,
		Supersedes: supersedes,
		TopicKey: redacted.TopicKey,
		Title: redacted.Title,
		Content: redacted.Content,
		Type: redacted.Type,
		Provenance: redacted.Provenance,
	})
	if err != nil {
		return SaveResult{}, fmt.Errorf("reserve memory governance record: %w", err)
	}

	result, err := p.Inner.Save(ctx, redacted)
	if err != nil {
		return SaveResult{
			Provider: p.Inner.ID(),
			Project: redacted.Project,
			Saved: false,
			GovernanceID: pending.GovernanceID,
			Summary: "provider delivery uncertain or failed; governance record remains pending",
		}, err
	}
	action := GovernanceSaved
	if supersedes != "" {
		action = GovernanceSuperseded
	}
	_, err = p.Store.Append(ctx, GovernanceEntry{
		Action: action,
		Project: redacted.Project,
		ReferenceID: pending.GovernanceID,
		Supersedes: supersedes,
		Provider: result.Provider,
		MemoryID: result.MemoryID,
	})
	if err != nil {
		result.GovernanceID = pending.GovernanceID
		result.Saved = false
		return result, fmt.Errorf("confirm memory governance record: %w", err)
	}
	result.GovernanceID = pending.GovernanceID
	return result, nil
}

func (p GovernedProvider) Review(ctx context.Context, project, governanceID string) (GovernedRecord, error) {
	record, err := p.Store.Record(project, governanceID)
	if err != nil {
		return GovernedRecord{}, err
	}
	if record.State == GovernanceDeletedState {
		return GovernedRecord{}, errors.New("deleted memory cannot be reviewed")
	}
	if _, err := p.Store.Append(ctx, GovernanceEntry{
		Action: GovernanceReviewed,
		Project: project,
		ReferenceID: governanceID,
	}); err != nil {
		return GovernedRecord{}, err
	}
	return p.Store.Record(project, governanceID)
}

func (p GovernedProvider) Delete(ctx context.Context, project, governanceID string, hard bool) (GovernedRecord, error) {
	record, err := p.Store.Record(project, governanceID)
	if err != nil {
		return GovernedRecord{}, err
	}
	if record.State != GovernanceActive {
		return GovernedRecord{}, fmt.Errorf("only active memory can be deleted, state=%s", record.State)
	}
	deleter, ok := p.Inner.(DeletableProjectMemory)
	if !ok {
		return GovernedRecord{}, errors.New("memory provider does not support governed deletion")
	}
	if strings.TrimSpace(record.MemoryID) == "" {
		return GovernedRecord{}, errors.New("governed memory has no provider memory id")
	}
	if _, err := deleter.Delete(ctx, DeleteRequest{
		Project: project, MemoryID: record.MemoryID, Hard: hard,
	}); err != nil {
		return GovernedRecord{}, err
	}
	if _, err := p.Store.Append(ctx, GovernanceEntry{
		Action: GovernanceDeleted,
		Project: project,
		ReferenceID: governanceID,
		HardDelete: hard,
	}); err != nil {
		return GovernedRecord{}, err
	}
	return p.Store.Record(project, governanceID)
}

func (p GovernedProvider) Prune(ctx context.Context, project string, olderThan time.Duration, hard bool) ([]GovernedRecord, error) {
	if olderThan <= 0 {
		return nil, errors.New("retention duration must be > 0")
	}
	records, err := p.Store.Records(project)
	if err != nil {
		return nil, err
	}
	cutoff := p.Store.now().UTC().Add(-olderThan)
	var deleted []GovernedRecord
	for _, record := range records {
		if record.State != GovernanceActive || !record.CreatedAt.Before(cutoff) {
			continue
		}
		value, err := p.Delete(ctx, project, record.GovernanceID, hard)
		if err != nil {
			return deleted, err
		}
		deleted = append(deleted, value)
	}
	return deleted, nil
}

var (
	privateKeyPattern = regexp.MustCompile(`(?s)-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----.*?-----END [A-Z0-9 ]*PRIVATE KEY-----`)
	bearerPattern = regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._~+/=-]{12,}`)
	secretAssignmentPattern = regexp.MustCompile(`(?i)\b(api[_-]?key|access[_-]?token|auth[_-]?token|secret|password|passwd)\s*[:=]\s*["']?([^\s"',;]{6,})["']?`)
	githubTokenPattern = regexp.MustCompile(`\b(?:ghp|gho|ghu|ghs|ghr)_[A-Za-z0-9]{20,}\b|\bgithub_pat_[A-Za-z0-9_]{20,}\b`)
	awsKeyPattern = regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`)
)

func RedactSaveRequest(request SaveRequest) SaveRequest {
	request.Title = RedactSensitive(request.Title)
	request.Content = RedactSensitive(request.Content)
	return request
}

func RedactSensitive(value string) string {
	value = privateKeyPattern.ReplaceAllString(value, "[REDACTED_PRIVATE_KEY]")
	value = bearerPattern.ReplaceAllString(value, "Bearer [REDACTED_TOKEN]")
	value = githubTokenPattern.ReplaceAllString(value, "[REDACTED_GITHUB_TOKEN]")
	value = awsKeyPattern.ReplaceAllString(value, "[REDACTED_AWS_ACCESS_KEY]")
	value = secretAssignmentPattern.ReplaceAllString(value, "$1=[REDACTED_SECRET]")
	return value
}

func governanceProjectKey(project string) string {
	sum := sha256.Sum256([]byte(project))
	return hex.EncodeToString(sum[:16])
}

func governanceRecordID(project, topic, title, content string, sequence uint64) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%d", project, topic, title, content, sequence)))
	return "memory-record-" + hex.EncodeToString(sum[:16])
}

func validGovernanceAction(action GovernanceAction) bool {
	switch action {
	case GovernancePending, GovernanceSaved, GovernanceSuperseded, GovernanceReviewed, GovernanceDeleted:
		return true
	default:
		return false
	}
}

func validateGovernanceEntry(entry GovernanceEntry) error {
	if entry.SchemaVersion != GovernanceSchemaVersion {
		return fmt.Errorf("unsupported memory governance schema version: %d", entry.SchemaVersion)
	}
	if err := validateProject(entry.Project); err != nil {
		return err
	}
	if entry.Sequence == 0 || entry.CreatedAt.IsZero() || !validGovernanceAction(entry.Action) {
		return errors.New("invalid memory governance entry")
	}
	if entry.Action == GovernancePending {
		if strings.TrimSpace(entry.GovernanceID) == "" ||
			strings.TrimSpace(entry.Title) == "" ||
			strings.TrimSpace(entry.Content) == "" ||
			!validType(entry.Type) {
			return errors.New("invalid governed memory pending entry")
		}
		sum := sha256.Sum256([]byte(entry.Content))
		if entry.ContentSHA256 != hex.EncodeToString(sum[:]) {
			return errors.New("memory governance content digest mismatch")
		}
	}
	if entry.Action == GovernanceSaved || entry.Action == GovernanceSuperseded {
		if strings.TrimSpace(entry.ReferenceID) == "" ||
			strings.TrimSpace(entry.Provider) == "" {
			return errors.New("invalid governed memory commit entry")
		}
	}
	if entry.Action == GovernanceReviewed || entry.Action == GovernanceDeleted {
		if strings.TrimSpace(entry.ReferenceID) == "" {
			return errors.New("invalid governed memory mutation entry")
		}
	}
	return nil
}
