package mutation

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/staging"
	"github.com/FernandoVillegas27/ai-engineering-os/core/taskworkspace"
)

type StageRuntimeResolver func(unit execution.WorkUnit) (staging.Runtime, error)

type StageConfig struct {
	MaxConcurrency int
	Policy         execution.SafeViewPolicy
}

type StagedSet struct {
	Workspaces  []taskworkspace.Workspace
	Inspections map[string]taskworkspace.Inspection
}

type stageResult struct {
	workspace  taskworkspace.Workspace
	inspection taskworkspace.Inspection
	err        error
}

func StageFrozenProposals(
	ctx context.Context,
	units []execution.WorkUnit,
	frozen []FrozenProposal,
	resolve StageRuntimeResolver,
	config StageConfig,
) (StagedSet, error) {
	if len(units) == 0 {
		return StagedSet{}, errors.New("frozen staging requires work units")
	}
	if len(frozen) == 0 {
		return StagedSet{}, errors.New("frozen staging requires proposals")
	}
	if resolve == nil {
		return StagedSet{}, errors.New("frozen staging requires runtime resolver")
	}
	if len(units) != len(frozen) {
		return StagedSet{}, errors.New("work-unit and frozen-proposal counts differ")
	}

	frozenByTask := map[string]FrozenProposal{}
	for _, item := range frozen {
		if _, exists := frozenByTask[item.TaskID]; exists {
			return StagedSet{}, fmt.Errorf("duplicate frozen proposal: %s", item.TaskID)
		}
		frozenByTask[item.TaskID] = item
	}

	seenUnits := map[string]bool{}
	for _, unit := range units {
		if seenUnits[unit.ID] {
			return StagedSet{}, fmt.Errorf("duplicate work unit: %s", unit.ID)
		}
		seenUnits[unit.ID] = true
		item, ok := frozenByTask[unit.ID]
		if !ok {
			return StagedSet{}, fmt.Errorf("missing frozen proposal for %s", unit.ID)
		}
		if item.AgentID != unit.AgentID {
			return StagedSet{}, fmt.Errorf(
				"agent mismatch for %s: unit=%s proposal=%s",
				unit.ID,
				unit.AgentID,
				item.AgentID,
			)
		}
	}

	maxConcurrency := config.MaxConcurrency
	if maxConcurrency <= 0 {
		maxConcurrency = 2
	}
	if maxConcurrency > 16 {
		maxConcurrency = 16
	}
	policy := config.Policy
	if len(policy.AllowedExtensions) == 0 && len(policy.AllowedBasenames) == 0 {
		policy = execution.DefaultSafeViewPolicy()
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	sem := make(chan struct{}, maxConcurrency)
	results := make(chan stageResult, len(units))

	for _, unit := range units {
		unit := unit
		item := frozenByTask[unit.ID]
		go func() {
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-runCtx.Done():
				results <- stageResult{err: runCtx.Err()}
				return
			}

			runtime, err := resolve(unit)
			if err != nil {
				results <- stageResult{err: fmt.Errorf("resolve stage runtime for %s: %w", unit.ID, err)}
				return
			}
			if runtime == nil {
				results <- stageResult{err: fmt.Errorf("stage runtime for %s is nil", unit.ID)}
				return
			}

			staged, err := runtime.Stage(runCtx, unit, cloneProposal(item.Proposal))
			if err != nil {
				results <- stageResult{err: fmt.Errorf("stage %s: %w", unit.ID, err)}
				return
			}
			workspace, inspection, err := adaptStageResult(unit, item, staged, policy)
			if err != nil {
				results <- stageResult{err: fmt.Errorf("validate staged result %s: %w", unit.ID, err)}
				return
			}
			results <- stageResult{workspace: workspace, inspection: inspection}
		}()
	}

	set := StagedSet{Inspections: map[string]taskworkspace.Inspection{}}
	var firstErr error
	for range units {
		result := <-results
		if result.err != nil {
			if firstErr == nil {
				firstErr = result.err
				cancel()
			}
			continue
		}
		set.Workspaces = append(set.Workspaces, result.workspace)
		set.Inspections[result.workspace.TaskID] = result.inspection
	}
	sort.Slice(set.Workspaces, func(i, j int) bool { return set.Workspaces[i].TaskID < set.Workspaces[j].TaskID })

	if firstErr != nil {
		return set, firstErr
	}
	if len(set.Workspaces) != len(units) {
		return set, errors.New("frozen staging returned incomplete workspace set")
	}
	if err := requireSharedPreImage(set.Workspaces); err != nil {
		return set, err
	}
	if err := taskworkspace.ValidateDisjoint(set.Workspaces); err != nil {
		return set, fmt.Errorf("staged workspace ownership conflict: %w", err)
	}
	return set, nil
}

func adaptStageResult(
	unit execution.WorkUnit,
	item FrozenProposal,
	staged staging.StageResult,
	policy execution.SafeViewPolicy,
) (taskworkspace.Workspace, taskworkspace.Inspection, error) {
	if staged.PreImage.IsZero() {
		return taskworkspace.Workspace{}, taskworkspace.Inspection{}, errors.New("stage result has no pre-image")
	}
	root, err := filepath.Abs(strings.TrimSpace(staged.Workspace))
	if err != nil {
		return taskworkspace.Workspace{}, taskworkspace.Inspection{}, fmt.Errorf("resolve stage workspace: %w", err)
	}
	info, err := os.Lstat(root)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return taskworkspace.Workspace{}, taskworkspace.Inspection{}, errors.New("stage workspace must be a regular directory")
	}
	projectRoot, err := filepath.Abs(unit.ProjectRoot)
	if err != nil {
		return taskworkspace.Workspace{}, taskworkspace.Inspection{}, fmt.Errorf("resolve project root: %w", err)
	}
	if projectInfo, statErr := os.Stat(projectRoot); statErr == nil && os.SameFile(projectInfo, info) {
		return taskworkspace.Workspace{}, taskworkspace.Inspection{}, errors.New("stage runtime returned the original project as workspace")
	}

	after, err := staging.SnapshotWorkspace(root, policy)
	if err != nil {
		return taskworkspace.Workspace{}, taskworkspace.Inspection{}, err
	}
	actual := staging.ChangesBetween(staged.PreImage.Snapshot(), after)
	reported := append([]staging.WorkspaceChange(nil), staged.Changes...)
	sortChanges(reported)
	if err := exactStageChanges(actual, reported); err != nil {
		return taskworkspace.Workspace{}, taskworkspace.Inspection{}, fmt.Errorf("stage runtime report mismatch: %w", err)
	}
	if err := staging.EnsureChangesMatchDeclared(actual, item.Declared); err != nil {
		return taskworkspace.Workspace{}, taskworkspace.Inspection{}, err
	}

	workspace := taskworkspace.Workspace{
		TaskID:   unit.ID,
		AgentID:  unit.AgentID,
		Root:     root,
		PreImage: staged.PreImage,
		Declared: append([]staging.WorkspaceChange(nil), item.Declared...),
	}
	inspection := taskworkspace.Inspection{
		Changes:     append([]staging.WorkspaceChange(nil), actual...),
		ResultImage: staging.NewPreImage(after),
	}
	return workspace, inspection, nil
}

func requireSharedPreImage(workspaces []taskworkspace.Workspace) error {
	expected := ""
	for _, workspace := range workspaces {
		digest := workspace.PreImage.Digest()
		if expected == "" {
			expected = digest
			continue
		}
		if digest != expected {
			return fmt.Errorf("staged task %s does not share the batch pre-image", workspace.TaskID)
		}
	}
	return nil
}

func sortChanges(values []staging.WorkspaceChange) {
	sort.Slice(values, func(i, j int) bool {
		if values[i].Path != values[j].Path {
			return values[i].Path < values[j].Path
		}
		return values[i].Action < values[j].Action
	})
}

func exactStageChanges(actual, reported []staging.WorkspaceChange) error {
	sortChanges(actual)
	sortChanges(reported)
	if len(actual) != len(reported) {
		return fmt.Errorf("change count mismatch: actual=%d reported=%d", len(actual), len(reported))
	}
	for i := range actual {
		if actual[i] != reported[i] {
			return fmt.Errorf(
				"change mismatch at %d: actual=%s:%s reported=%s:%s",
				i,
				actual[i].Action,
				actual[i].Path,
				reported[i].Action,
				reported[i].Path,
			)
		}
	}
	return nil
}
