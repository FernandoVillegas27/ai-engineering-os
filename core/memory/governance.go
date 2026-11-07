package memory

import (
	"crypto/rand"
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
)

const GovernanceSchemaVersion = 1

type RecordStatus string

const (
	RecordActive     RecordStatus = "active"
	RecordDeleted    RecordStatus = "deleted"
	RecordSuperseded RecordStatus = "superseded"
)

type Provenance struct {
	RunID         string `json:"run_id,omitempty"`
	ArtifactID    string `json:"artifact_id,omitempty"`
	TaskID        string `json:"task_id,omitempty"`
	ProducerAgent string `json:"producer_agent,omitempty"`
	Source        string `json:"source,omitempty"`
}

type Record struct {
	SchemaVersion int          `json:"schema_version"`
	ID            string       `json:"id"`
	Project       string       `json:"project"`
	Title         string       `json:"title"`
	Content       string       `json:"content"`
	Type          Type         `json:"type"`
	TopicKey      string       `json:"topic_key,omitempty"`
	Provider      string       `json:"provider,omitempty"`
	Provenance    Provenance   `json:"provenance"`
	Status        RecordStatus `json:"status"`
	CreatedAt     time.Time    `json:"created_at"`
	UpdatedAt     time.Time    `json:"updated_at"`
	ExpiresAt     *time.Time   `json:"expires_at,omitempty"`
	DeletedAt     *time.Time   `json:"deleted_at,omitempty"`
	DeleteReason  string       `json:"delete_reason,omitempty"`
	SupersededBy  string       `json:"superseded_by,omitempty"`
	ContentSHA256 string       `json:"content_sha256"`
}

type GovernedSaveRequest struct {
	SaveRequest
	Provider   string
	Provenance Provenance
	Retention  time.Duration
}

type GovernanceStore struct {
	root string
}

func NewGovernanceStore(root string) (*GovernanceStore, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, errors.New("memory governance root is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve memory governance root: %w", err)
	}
	if err := os.MkdirAll(absolute, 0o700); err != nil {
		return nil, fmt.Errorf("create memory governance root: %w", err)
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, errors.New("memory governance root must be a regular directory")
	}
	return &GovernanceStore{root: absolute}, nil
}

func (s *GovernanceStore) Save(request GovernedSaveRequest, now time.Time) (Record, error) {
	if s == nil {
		return Record{}, errors.New("memory governance store is required")
	}
	if err := ValidateSaveRequest(request.SaveRequest); err != nil {
		return Record{}, err
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	now = now.UTC()
	title, _ := RedactSensitive(request.Title)
	content, _ := RedactSensitive(request.Content)
	if strings.TrimSpace(title) == "" || strings.TrimSpace(content) == "" {
		return Record{}, errors.New("memory becomes empty after sensitive-content redaction")
	}
	id, err := newMemoryRecordID()
	if err != nil {
		return Record{}, err
	}
	record := Record{
		SchemaVersion: GovernanceSchemaVersion,
		ID:            id,
		Project:       strings.TrimSpace(request.Project),
		Title:         title,
		Content:       content,
		Type:          request.Type,
		TopicKey:      strings.TrimSpace(request.TopicKey),
		Provider:      strings.TrimSpace(request.Provider),
		Provenance:    normalizeProvenance(request.Provenance),
		Status:        RecordActive,
		CreatedAt:     now,
		UpdatedAt:     now,
		ContentSHA256: contentDigest(content),
	}
	if request.Retention > 0 {
		expires := now.Add(request.Retention).UTC()
		record.ExpiresAt = &expires
	}
	if err := validateRecord(record); err != nil {
		return Record{}, err
	}
	projectDir, err := s.projectDir(record.Project)
	if err != nil {
		return Record{}, err
	}
	if err := os.MkdirAll(projectDir, 0o700); err != nil {
		return Record{}, err
	}
	if err := writeRecordExclusive(filepath.Join(projectDir, record.ID+".json"), record); err != nil {
		return Record{}, err
	}
	return record, nil
}

func (s *GovernanceStore) List(project string, includeInactive bool, now time.Time) ([]Record, error) {
	if s == nil {
		return nil, errors.New("memory governance store is required")
	}
	if err := validateProject(project); err != nil {
		return nil, err
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	dir, err := s.projectDir(project)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return []Record{}, nil
	}
	if err != nil {
		return nil, err
	}
	values := make([]Record, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		record, err := s.loadPath(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		if record.Project != strings.TrimSpace(project) {
			return nil, fmt.Errorf("memory project boundary mismatch for %s", record.ID)
		}
		if !includeInactive && !recordUsable(record, now) {
			continue
		}
		values = append(values, record)
	}
	sort.Slice(values, func(i, j int) bool {
		if !values[i].UpdatedAt.Equal(values[j].UpdatedAt) {
			return values[i].UpdatedAt.After(values[j].UpdatedAt)
		}
		return values[i].ID < values[j].ID
	})
	return values, nil
}

func (s *GovernanceStore) Load(project, id string) (Record, error) {
	if err := validateProject(project); err != nil {
		return Record{}, err
	}
	if !validRecordID(id) {
		return Record{}, errors.New("invalid memory record id")
	}
	dir, err := s.projectDir(project)
	if err != nil {
		return Record{}, err
	}
	record, err := s.loadPath(filepath.Join(dir, id+".json"))
	if err != nil {
		return Record{}, err
	}
	if record.Project != strings.TrimSpace(project) {
		return Record{}, errors.New("memory record belongs to another project")
	}
	return record, nil
}

func (s *GovernanceStore) Delete(project, id, reason string, now time.Time) (Record, error) {
	record, err := s.Load(project, id)
	if err != nil {
		return Record{}, err
	}
	if record.Status == RecordDeleted {
		return record, nil
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	now = now.UTC()
	record.Status = RecordDeleted
	record.UpdatedAt = now
	record.DeletedAt = &now
	record.DeleteReason = truncateUTF8Bytes(strings.TrimSpace(reason), 1000)
	if err := s.replace(record); err != nil {
		return Record{}, err
	}
	return record, nil
}

func (s *GovernanceStore) ActiveByTopic(project, topic string, now time.Time) (Record, bool, error) {
	topic = strings.TrimSpace(topic)
	if topic == "" {
		return Record{}, false, nil
	}
	values, err := s.List(project, false, now)
	if err != nil {
		return Record{}, false, err
	}
	for _, record := range values {
		if record.TopicKey == topic {
			return record, true, nil
		}
	}
	return Record{}, false, nil
}

func (s *GovernanceStore) Supersede(project, id string, replacement GovernedSaveRequest, now time.Time) (Record, Record, error) {
	current, err := s.Load(project, id)
	if err != nil {
		return Record{}, Record{}, err
	}
	if current.Status != RecordActive {
		return Record{}, Record{}, errors.New("only active memory can be superseded")
	}
	replacement.Project = strings.TrimSpace(project)
	if strings.TrimSpace(replacement.TopicKey) == "" {
		replacement.TopicKey = current.TopicKey
	}
	next, err := s.Save(replacement, now)
	if err != nil {
		return Record{}, Record{}, err
	}
	current.Status = RecordSuperseded
	current.SupersededBy = next.ID
	current.UpdatedAt = next.CreatedAt
	if err := s.replace(current); err != nil {
		dir, dirErr := s.projectDir(project)
		if dirErr == nil {
			_ = os.Remove(filepath.Join(dir, next.ID+".json"))
		}
		return Record{}, Record{}, err
	}
	return current, next, nil
}

func (s *GovernanceStore) Recall(project, query string, limit int, now time.Time) ([]Record, error) {
	if err := ValidateRecallRequest(RecallRequest{Project: project, Query: query, Limit: limit}); err != nil {
		return nil, err
	}
	if limit == 0 {
		limit = DefaultRecallLimit
	}
	values, err := s.List(project, false, now)
	if err != nil {
		return nil, err
	}
	terms := tokenize(query)
	type scored struct {
		record Record
		score  int
	}
	var matches []scored
	for _, record := range values {
		haystack := strings.ToLower(record.Title + "\n" + record.Content + "\n" + record.TopicKey)
		score := 0
		for _, term := range terms {
			if strings.Contains(haystack, term) {
				score++
			}
		}
		if score == 0 && len(terms) > 0 {
			continue
		}
		matches = append(matches, scored{record: record, score: score})
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score > matches[j].score
		}
		if !matches[i].record.UpdatedAt.Equal(matches[j].record.UpdatedAt) {
			return matches[i].record.UpdatedAt.After(matches[j].record.UpdatedAt)
		}
		return matches[i].record.ID < matches[j].record.ID
	})
	if len(matches) > limit {
		matches = matches[:limit]
	}
	out := make([]Record, 0, len(matches))
	for _, match := range matches {
		out = append(out, match.record)
	}
	return out, nil
}

func (s *GovernanceStore) replace(record Record) error {
	if err := validateRecord(record); err != nil {
		return err
	}
	dir, err := s.projectDir(record.Project)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, record.ID+".json")
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errors.New("memory record must be a regular file")
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	temp := path + ".tmp"
	if err := os.WriteFile(temp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(temp, path); err != nil {
		_ = os.Remove(temp)
		return err
	}
	return nil
}

func (s *GovernanceStore) loadPath(path string) (Record, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return Record{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return Record{}, errors.New("memory record must be a regular file")
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
	if err := validateRecord(record); err != nil {
		return Record{}, err
	}
	return record, nil
}

func (s *GovernanceStore) projectDir(project string) (string, error) {
	if err := validateProject(project); err != nil {
		return "", err
	}
	path := filepath.Join(s.root, projectDirectoryName(project))
	rel, err := filepath.Rel(filepath.Clean(s.root), filepath.Clean(path))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("memory project path escapes governance root")
	}
	return filepath.Clean(path), nil
}

func projectDirectoryName(project string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(project)))
	return hex.EncodeToString(sum[:12])
}

func writeRecordExclusive(path string, record Record) error {
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
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

func validateRecord(record Record) error {
	if record.SchemaVersion != GovernanceSchemaVersion {
		return fmt.Errorf("unsupported memory governance schema version: %d", record.SchemaVersion)
	}
	if !validRecordID(record.ID) {
		return errors.New("invalid memory record id")
	}
	if err := ValidateSaveRequest(SaveRequest{
		Project: record.Project,
		Title: record.Title,
		Content: record.Content,
		Type: record.Type,
		TopicKey: record.TopicKey,
	}); err != nil {
		return err
	}
	switch record.Status {
	case RecordActive, RecordDeleted, RecordSuperseded:
	default:
		return fmt.Errorf("invalid memory status: %q", record.Status)
	}
	if record.CreatedAt.IsZero() || record.UpdatedAt.IsZero() {
		return errors.New("memory timestamps are required")
	}
	if record.ContentSHA256 != contentDigest(record.Content) {
		return errors.New("memory content digest mismatch")
	}
	if record.Status == RecordSuperseded && !validRecordID(record.SupersededBy) {
		return errors.New("superseded memory requires superseded_by")
	}
	return nil
}

func recordUsable(record Record, now time.Time) bool {
	if record.Status != RecordActive {
		return false
	}
	return record.ExpiresAt == nil || now.Before(*record.ExpiresAt)
}

func normalizeProvenance(value Provenance) Provenance {
	value.RunID = truncateUTF8Bytes(strings.TrimSpace(value.RunID), 300)
	value.ArtifactID = truncateUTF8Bytes(strings.TrimSpace(value.ArtifactID), 300)
	value.TaskID = truncateUTF8Bytes(strings.TrimSpace(value.TaskID), 300)
	value.ProducerAgent = truncateUTF8Bytes(strings.TrimSpace(value.ProducerAgent), 300)
	value.Source = truncateUTF8Bytes(strings.TrimSpace(value.Source), 300)
	return value
}

func contentDigest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func newMemoryRecordID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return "memory-" + hex.EncodeToString(raw[:]), nil
}

func validRecordID(value string) bool {
	if !strings.HasPrefix(value, "memory-") || len(value) != len("memory-")+32 {
		return false
	}
	for _, r := range strings.TrimPrefix(value, "memory-") {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}

func tokenize(value string) []string {
	fields := strings.Fields(strings.ToLower(value))
	seen := map[string]bool{}
	var out []string
	for _, field := range fields {
		field = strings.Trim(field, ".,:;!?()[]{}<>\"'")
		if len(field) < 2 || seen[field] {
			continue
		}
		seen[field] = true
		out = append(out, field)
	}
	return out
}

var sensitivePatterns = []*regexp.Regexp{
	regexp.MustCompile("(?i)(authorization\\s*:\\s*bearer\\s+)[A-Za-z0-9._~+/=-]+"),
	regexp.MustCompile("(?i)((?:api[_-]?key|token|secret|password|passwd|pwd)\\s*[:=]\\s*)[^\\s,;]+"),
	regexp.MustCompile("\\bgh[pousr]_[A-Za-z0-9]{20,}\\b"),
	regexp.MustCompile("\\bsk-[A-Za-z0-9_-]{16,}\\b"),
	regexp.MustCompile("(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----"),
}

func RedactSensitive(value string) (string, int) {
	result := value
	count := 0
	for index, pattern := range sensitivePatterns {
		replacement := "[REDACTED]"
		if index < 2 {
			replacement = "$1[REDACTED]"
		}
		next := pattern.ReplaceAllString(result, replacement)
		if next != result {
			count++
			result = next
		}
	}
	return result, count
}
