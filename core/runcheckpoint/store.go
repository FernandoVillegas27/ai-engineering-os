package runcheckpoint

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/integration"
	"github.com/FernandoVillegas27/ai-engineering-os/core/staging"
)

const SchemaVersion = 1

type Phase string

const (
	PhaseSourceIntegrated Phase = "source-integrated"
	PhaseFinalIntegrated  Phase = "final-integrated"
)

type Record struct {
	SchemaVersion   int                       `json:"schema_version"`
	RunID           string                    `json:"run_id"`
	Phase           Phase                     `json:"phase"`
	OriginalRoot    string                    `json:"original_root"`
	PreImageDigest  string                    `json:"pre_image_digest"`
	PreImageEntries map[string]string         `json:"pre_image_entries"`
	WorkspaceDigest string                    `json:"workspace_digest"`
	Changes         []staging.WorkspaceChange `json:"changes"`
	CreatedAt       time.Time                 `json:"created_at"`
}

type Store struct {
	root   string
	policy execution.SafeViewPolicy
}

type Config struct {
	Root   string
	Policy execution.SafeViewPolicy
}

func NewStore(config Config) (*Store, error) {
	root := strings.TrimSpace(config.Root)
	if root == "" {
		return nil, errors.New("run checkpoint store root is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve run checkpoint root: %w", err)
	}
	if err := os.MkdirAll(absolute, 0o700); err != nil {
		return nil, fmt.Errorf("create run checkpoint root: %w", err)
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, errors.New("run checkpoint root must be a regular directory")
	}
	policy := config.Policy
	if len(policy.AllowedExtensions) == 0 && len(policy.AllowedBasenames) == 0 {
		policy = execution.DefaultSafeViewPolicy()
	}
	return &Store{root: absolute, policy: policy}, nil
}

func (s *Store) Save(
	ctx context.Context,
	runID string,
	phase Phase,
	originalRoot string,
	workspace integration.IntegratedWorkspace,
	now time.Time,
) (Record, error) {
	if s == nil {
		return Record{}, errors.New("run checkpoint store is required")
	}
	if err := ctx.Err(); err != nil {
		return Record{}, err
	}
	if !safeToken(runID) {
		return Record{}, errors.New("run checkpoint run id is unsafe")
	}
	if !validPhase(phase) {
		return Record{}, fmt.Errorf("unsupported run checkpoint phase: %s", phase)
	}
	if workspace.PreImage.IsZero() || strings.TrimSpace(workspace.Root) == "" {
		return Record{}, errors.New("run checkpoint requires integrated workspace and pre-image")
	}
	originalRoot, err := filepath.Abs(strings.TrimSpace(originalRoot))
	if err != nil {
		return Record{}, fmt.Errorf("resolve original root: %w", err)
	}
	if info, err := os.Lstat(originalRoot); err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return Record{}, errors.New("original root must be a regular directory")
	}

	workspaceSnapshot, err := staging.SnapshotWorkspace(workspace.Root, s.policy)
	if err != nil {
		return Record{}, fmt.Errorf("snapshot checkpoint source workspace: %w", err)
	}
	actualChanges := staging.ChangesBetween(workspace.PreImage.Snapshot(), workspaceSnapshot)
	if err := equalChanges(actualChanges, workspace.Changes); err != nil {
		return Record{}, fmt.Errorf("checkpoint source workspace does not match declared integrated changes: %w", err)
	}

	if err := verifyOriginalMatchesPreImage(ctx, originalRoot, workspace.PreImage, s.policy, s.root); err != nil {
		return Record{}, err
	}

	if now.IsZero() {
		now = time.Now().UTC()
	}
	record := Record{
		SchemaVersion:   SchemaVersion,
		RunID:           runID,
		Phase:           phase,
		OriginalRoot:    originalRoot,
		PreImageDigest:  workspace.PreImage.Digest(),
		PreImageEntries: encodeSnapshot(workspace.PreImage.Snapshot()),
		WorkspaceDigest: workspaceSnapshot.Digest(),
		Changes:         sortedChanges(workspace.Changes),
		CreatedAt:       now.UTC(),
	}
	if err := Validate(record); err != nil {
		return Record{}, err
	}

	runDir, err := s.ensureRunDir(runID)
	if err != nil {
		return Record{}, err
	}
	target := filepath.Join(runDir, string(phase))
	if _, err := os.Lstat(target); err == nil {
		return Record{}, fmt.Errorf("run checkpoint already exists: %s/%s", runID, phase)
	} else if !os.IsNotExist(err) {
		return Record{}, err
	}

	temp, err := os.MkdirTemp(runDir, ".checkpoint-*")
	if err != nil {
		return Record{}, fmt.Errorf("create checkpoint temp directory: %w", err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(temp)
		}
	}()

	checkpointWorkspace := filepath.Join(temp, "workspace")
	if _, err := execution.PrepareSafeReadOnlyViewWithPolicy(workspace.Root, checkpointWorkspace, s.policy); err != nil {
		return Record{}, fmt.Errorf("copy checkpoint workspace: %w", err)
	}
	copied, err := staging.SnapshotWorkspace(checkpointWorkspace, s.policy)
	if err != nil {
		return Record{}, err
	}
	if copied.Digest() != record.WorkspaceDigest {
		return Record{}, errors.New("checkpoint workspace copy digest mismatch")
	}

	manifest, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return Record{}, err
	}
	manifest = append(manifest, '\n')
	if err := os.WriteFile(filepath.Join(temp, "manifest.json"), manifest, 0o600); err != nil {
		return Record{}, fmt.Errorf("write checkpoint manifest: %w", err)
	}
	if err := os.Rename(temp, target); err != nil {
		return Record{}, fmt.Errorf("commit checkpoint: %w", err)
	}
	cleanup = false
	return record, nil
}

func (s *Store) Load(runID string, phase Phase) (Record, error) {
	if s == nil {
		return Record{}, errors.New("run checkpoint store is required")
	}
	dir, err := s.phaseDir(runID, phase)
	if err != nil {
		return Record{}, err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return Record{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return Record{}, errors.New("checkpoint phase directory must be regular")
	}
	manifestPath := filepath.Join(dir, "manifest.json")
	info, err = os.Lstat(manifestPath)
	if err != nil {
		return Record{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return Record{}, errors.New("checkpoint manifest must be a regular file")
	}
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return Record{}, err
	}
	var record Record
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&record); err != nil {
		return Record{}, fmt.Errorf("decode checkpoint manifest: %w", err)
	}
	if record.RunID != runID || record.Phase != phase {
		return Record{}, errors.New("checkpoint manifest identity mismatch")
	}
	if err := Validate(record); err != nil {
		return Record{}, err
	}
	return record, nil
}

func (s *Store) Restore(
	ctx context.Context,
	runID string,
	phase Phase,
	parent string,
) (integration.IntegratedWorkspace, error) {
	record, err := s.Load(runID, phase)
	if err != nil {
		return integration.IntegratedWorkspace{}, err
	}
	preImageSnapshot, err := decodeSnapshot(record.PreImageEntries)
	if err != nil {
		return integration.IntegratedWorkspace{}, err
	}
	preImage := staging.NewPreImage(preImageSnapshot)
	if err := verifyOriginalMatchesPreImage(ctx, record.OriginalRoot, preImage, s.policy, s.root); err != nil {
		return integration.IntegratedWorkspace{}, err
	}

	phaseDir, err := s.phaseDir(runID, phase)
	if err != nil {
		return integration.IntegratedWorkspace{}, err
	}
	source := filepath.Join(phaseDir, "workspace")
	info, err := os.Lstat(source)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return integration.IntegratedWorkspace{}, errors.New("checkpoint workspace must be a regular directory")
	}
	current, err := staging.SnapshotWorkspace(source, s.policy)
	if err != nil {
		return integration.IntegratedWorkspace{}, err
	}
	if current.Digest() != record.WorkspaceDigest {
		return integration.IntegratedWorkspace{}, errors.New("checkpoint workspace digest mismatch")
	}
	actualChanges := staging.ChangesBetween(preImageSnapshot, current)
	if err := equalChanges(actualChanges, record.Changes); err != nil {
		return integration.IntegratedWorkspace{}, fmt.Errorf("checkpoint change-set mismatch: %w", err)
	}

	parent = strings.TrimSpace(parent)
	if parent == "" {
		parent = filepath.Dir(record.OriginalRoot)
	}
	parent, err = filepath.Abs(parent)
	if err != nil {
		return integration.IntegratedWorkspace{}, fmt.Errorf("resolve checkpoint restore parent: %w", err)
	}
	info, err = os.Lstat(parent)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return integration.IntegratedWorkspace{}, errors.New("checkpoint restore parent must be a regular directory")
	}

	dir, err := os.MkdirTemp(parent, ".ai-engineering-os-integration-resume-*")
	if err != nil {
		return integration.IntegratedWorkspace{}, fmt.Errorf("create checkpoint restore workspace: %w", err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(dir)
		}
	}()
	if _, err := execution.PrepareSafeReadOnlyViewWithPolicy(source, dir, s.policy); err != nil {
		return integration.IntegratedWorkspace{}, err
	}
	restored, err := staging.SnapshotWorkspace(dir, s.policy)
	if err != nil {
		return integration.IntegratedWorkspace{}, err
	}
	if restored.Digest() != record.WorkspaceDigest {
		return integration.IntegratedWorkspace{}, errors.New("restored checkpoint digest mismatch")
	}
	if err := ctx.Err(); err != nil {
		return integration.IntegratedWorkspace{}, err
	}

	cleanup = false
	return integration.IntegratedWorkspace{
		Root:     dir,
		PreImage: preImage,
		Changes:  append([]staging.WorkspaceChange(nil), record.Changes...),
	}, nil
}

func Validate(record Record) error {
	if record.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported checkpoint schema version: %d", record.SchemaVersion)
	}
	if !safeToken(record.RunID) || !validPhase(record.Phase) {
		return errors.New("checkpoint identity is invalid")
	}
	if strings.TrimSpace(record.OriginalRoot) == "" {
		return errors.New("checkpoint original root is required")
	}
	if record.PreImageDigest == "" || len(record.PreImageEntries) == 0 {
		return errors.New("checkpoint pre-image is required")
	}
	snapshot, err := decodeSnapshot(record.PreImageEntries)
	if err != nil {
		return err
	}
	if snapshot.Digest() != record.PreImageDigest {
		return errors.New("checkpoint pre-image digest mismatch")
	}
	if record.WorkspaceDigest == "" {
		return errors.New("checkpoint workspace digest is required")
	}
	if len(record.Changes) == 0 {
		return errors.New("checkpoint changes are required")
	}
	for _, change := range record.Changes {
		if strings.TrimSpace(change.Path) == "" {
			return errors.New("checkpoint change path is required")
		}
		switch change.Action {
		case "create", "modify", "delete":
		default:
			return fmt.Errorf("unsupported checkpoint action: %s", change.Action)
		}
	}
	if record.CreatedAt.IsZero() {
		return errors.New("checkpoint created_at is required")
	}
	return nil
}

func (s *Store) ensureRunDir(runID string) (string, error) {
	if !safeToken(runID) {
		return "", errors.New("run checkpoint run id is unsafe")
	}
	dir := filepath.Join(s.root, runID)
	if err := os.Mkdir(dir, 0o700); err != nil && !os.IsExist(err) {
		return "", err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", errors.New("run checkpoint run directory must be regular")
	}
	return dir, nil
}

func (s *Store) phaseDir(runID string, phase Phase) (string, error) {
	if !safeToken(runID) || !validPhase(phase) {
		return "", errors.New("checkpoint identity is unsafe")
	}
	return filepath.Join(s.root, runID, string(phase)), nil
}

func verifyOriginalMatchesPreImage(
	ctx context.Context,
	originalRoot string,
	preImage staging.PreImage,
	policy execution.SafeViewPolicy,
	tempParent string,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	temp, err := os.MkdirTemp(tempParent, ".checkpoint-original-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temp)
	if _, err := execution.PrepareSafeReadOnlyViewWithPolicy(originalRoot, temp, policy); err != nil {
		return fmt.Errorf("prepare original checkpoint view: %w", err)
	}
	snapshot, err := staging.SnapshotWorkspace(temp, policy)
	if err != nil {
		return err
	}
	if snapshot.Digest() != preImage.Digest() {
		return errors.New("original project no longer matches checkpoint pre-image")
	}
	return nil
}

func encodeSnapshot(snapshot staging.WorkspaceSnapshot) map[string]string {
	out := make(map[string]string, len(snapshot))
	for path, hash := range snapshot {
		out[path] = hex.EncodeToString(hash[:])
	}
	return out
}

func decodeSnapshot(entries map[string]string) (staging.WorkspaceSnapshot, error) {
	snapshot := staging.WorkspaceSnapshot{}
	for path, encoded := range entries {
		if strings.TrimSpace(path) == "" {
			return nil, errors.New("checkpoint pre-image contains empty path")
		}
		data, err := hex.DecodeString(encoded)
		if err != nil || len(data) != 32 {
			return nil, fmt.Errorf("invalid checkpoint hash for %s", path)
		}
		var hash [32]byte
		copy(hash[:], data)
		snapshot[path] = hash
	}
	return snapshot, nil
}

func equalChanges(actual, expected []staging.WorkspaceChange) error {
	left := sortedChanges(actual)
	right := sortedChanges(expected)
	if len(left) != len(right) {
		return fmt.Errorf("change count mismatch: actual=%d expected=%d", len(left), len(right))
	}
	for i := range left {
		if left[i] != right[i] {
			return fmt.Errorf(
				"change mismatch at %d: actual=%s:%s expected=%s:%s",
				i,
				left[i].Action,
				left[i].Path,
				right[i].Action,
				right[i].Path,
			)
		}
	}
	return nil
}

func sortedChanges(values []staging.WorkspaceChange) []staging.WorkspaceChange {
	out := append([]staging.WorkspaceChange(nil), values...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return out[i].Action < out[j].Action
	})
	return out
}

func validPhase(value Phase) bool {
	switch value {
	case PhaseSourceIntegrated, PhaseFinalIntegrated:
		return true
	default:
		return false
	}
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
