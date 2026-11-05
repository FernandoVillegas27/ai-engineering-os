package taskworkspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/proposal"
	"github.com/FernandoVillegas27/ai-engineering-os/core/staging"
)

type Config struct {
	OriginalRoot string
	Parent       string
	Policy       execution.SafeViewPolicy
}

type Manager struct {
	root   string
	parent string
	policy execution.SafeViewPolicy
}

type Workspace struct {
	TaskID    string
	AgentID   string
	Root      string
	PreImage  staging.PreImage
	Declared  []staging.WorkspaceChange
	Copied    int
	Excluded  []string
}

type Inspection struct {
	Changes     []staging.WorkspaceChange
	ResultImage staging.PreImage
}

func New(config Config) (*Manager, error) {
	root, err := filepath.Abs(strings.TrimSpace(config.OriginalRoot))
	if err != nil {
		return nil, fmt.Errorf("resolve original root: %w", err)
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return nil, errors.New("original root must be an existing directory")
	}

	parent := strings.TrimSpace(config.Parent)
	if parent == "" {
		parent = filepath.Dir(root)
	}
	parent, err = filepath.Abs(parent)
	if err != nil {
		return nil, fmt.Errorf("resolve task workspace parent: %w", err)
	}
	parentInfo, err := os.Stat(parent)
	if err != nil || !parentInfo.IsDir() {
		return nil, errors.New("task workspace parent must be an existing directory")
	}
	if within(root, parent) {
		return nil, errors.New("task workspace parent must be outside the original project")
	}

	policy := config.Policy
	if len(policy.AllowedExtensions) == 0 && len(policy.AllowedBasenames) == 0 {
		policy = execution.DefaultSafeViewPolicy()
	}
	return &Manager{root: root, parent: parent, policy: policy}, nil
}

func (m *Manager) Prepare(
	ctx context.Context,
	taskID string,
	agentID string,
	declared []staging.WorkspaceChange,
) (Workspace, error) {
	if m == nil {
		return Workspace{}, errors.New("task workspace manager is required")
	}
	if err := ctx.Err(); err != nil {
		return Workspace{}, fmt.Errorf("prepare task workspace: %w", err)
	}
	taskID = strings.TrimSpace(taskID)
	agentID = strings.TrimSpace(agentID)
	if taskID == "" || agentID == "" {
		return Workspace{}, errors.New("task id and agent id are required")
	}

	normalized, err := normalizeDeclared(declared, m.policy)
	if err != nil {
		return Workspace{}, err
	}

	dir, err := os.MkdirTemp(m.parent, ".ai-engineering-os-task-*")
	if err != nil {
		return Workspace{}, fmt.Errorf("create task workspace: %w", err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(dir)
		}
	}()

	view, err := execution.PrepareSafeReadOnlyViewWithPolicy(m.root, dir, m.policy)
	if err != nil {
		return Workspace{}, err
	}
	snapshot, err := staging.SnapshotWorkspace(dir, m.policy)
	if err != nil {
		return Workspace{}, err
	}
	preImage := staging.NewPreImage(snapshot)
	if err := validateDeclaredAgainstPreImage(normalized, preImage); err != nil {
		return Workspace{}, err
	}
	if err := ctx.Err(); err != nil {
		return Workspace{}, fmt.Errorf("prepare task workspace: %w", err)
	}

	cleanup = false
	return Workspace{
		TaskID:   taskID,
		AgentID:  agentID,
		Root:     dir,
		PreImage: preImage,
		Declared: append([]staging.WorkspaceChange(nil), normalized...),
		Copied:   view.CopiedFiles,
		Excluded: append([]string(nil), view.ExcludedPaths...),
	}, nil
}

func (m *Manager) Inspect(ctx context.Context, workspace Workspace) (Inspection, error) {
	if m == nil {
		return Inspection{}, errors.New("task workspace manager is required")
	}
	if err := ctx.Err(); err != nil {
		return Inspection{}, fmt.Errorf("inspect task workspace: %w", err)
	}
	if workspace.PreImage.IsZero() {
		return Inspection{}, errors.New("task workspace pre-image is required")
	}
	root, err := filepath.Abs(workspace.Root)
	if err != nil {
		return Inspection{}, fmt.Errorf("resolve task workspace root: %w", err)
	}
	if !within(m.parent, root) || root == m.parent || root == m.root || within(m.root, root) {
		return Inspection{}, errors.New("task workspace is outside the isolated workspace boundary")
	}
	info, err := os.Lstat(root)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return Inspection{}, errors.New("task workspace root must be a regular directory")
	}

	after, err := staging.SnapshotWorkspace(root, m.policy)
	if err != nil {
		return Inspection{}, err
	}
	resultImage := staging.NewPreImage(after)
	changes := staging.ChangesBetween(workspace.PreImage.Snapshot(), after)
	if len(changes) == 0 {
		return Inspection{
			Changes:     []staging.WorkspaceChange{},
			ResultImage: resultImage,
		}, nil
	}
	if err := staging.EnsureChangesMatchDeclared(changes, workspace.Declared); err != nil {
		return Inspection{}, err
	}
	return Inspection{
		Changes:     append([]staging.WorkspaceChange(nil), changes...),
		ResultImage: resultImage,
	}, nil
}

func (m *Manager) Cleanup(workspace Workspace) error {
	if m == nil {
		return errors.New("task workspace manager is required")
	}
	root, err := filepath.Abs(workspace.Root)
	if err != nil {
		return fmt.Errorf("resolve task workspace root: %w", err)
	}
	if !within(m.parent, root) || root == m.parent || root == m.root || within(m.root, root) {
		return errors.New("refusing to clean a path outside isolated task workspace boundary")
	}
	return os.RemoveAll(root)
}

func ValidateDisjoint(workspaces []Workspace) error {
	owners := map[string]string{}
	for _, workspace := range workspaces {
		for _, change := range workspace.Declared {
			if owner, exists := owners[change.Path]; exists {
				return fmt.Errorf(
					"task workspaces %s and %s both declare %s",
					owner,
					workspace.TaskID,
					change.Path,
				)
			}
			owners[change.Path] = workspace.TaskID
		}
	}
	return nil
}

func normalizeDeclared(values []staging.WorkspaceChange, policy execution.SafeViewPolicy) ([]staging.WorkspaceChange, error) {
	if len(values) == 0 {
		return nil, errors.New("task workspace requires declared file changes")
	}
	files := make([]proposal.FileChange, 0, len(values))
	for _, value := range values {
		files = append(files, proposal.FileChange{Path: value.Path, Action: value.Action})
	}
	return staging.ValidateDeclaredFiles(files, policy)
}

func validateDeclaredAgainstPreImage(values []staging.WorkspaceChange, preImage staging.PreImage) error {
	snapshot := preImage.Snapshot()
	for _, value := range values {
		_, exists := snapshot[value.Path]
		switch value.Action {
		case "create":
			if exists {
				return fmt.Errorf("declared create path already exists in pre-image: %s", value.Path)
			}
		case "modify", "delete":
			if !exists {
				return fmt.Errorf("declared %s path does not exist in pre-image: %s", value.Action, value.Path)
			}
		default:
			return fmt.Errorf("unsupported declared action %q", value.Action)
		}
	}
	return nil
}

func within(root, candidate string) bool {
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func SortedDeclared(workspace Workspace) []staging.WorkspaceChange {
	out := append([]staging.WorkspaceChange(nil), workspace.Declared...)
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}
