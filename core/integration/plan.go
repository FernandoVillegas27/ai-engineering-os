package integration

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/FernandoVillegas27/ai-engineering-os/core/staging"
	"github.com/FernandoVillegas27/ai-engineering-os/core/taskworkspace"
)

type PlannedChange struct {
	TaskID  string `json:"task_id"`
	AgentID string `json:"agent_id"`
	Path    string `json:"path"`
	Action  string `json:"action"`
}

type Plan struct {
	PreImageDigest string          `json:"pre_image_digest"`
	Changes        []PlannedChange `json:"changes"`
}

func Build(workspaces []taskworkspace.Workspace, inspections map[string]taskworkspace.Inspection) (Plan, error) {
	if len(workspaces) == 0 {
		return Plan{}, errors.New("integration plan requires at least one task workspace")
	}
	if len(inspections) == 0 {
		return Plan{}, errors.New("integration plan requires inspected task workspaces")
	}
	if err := taskworkspace.ValidateDisjoint(workspaces); err != nil {
		return Plan{}, fmt.Errorf("declared task workspace conflict: %w", err)
	}

	expectedDigest := ""
	seenTasks := map[string]bool{}
	var changes []PlannedChange

	for _, workspace := range workspaces {
		taskID := strings.TrimSpace(workspace.TaskID)
		agentID := strings.TrimSpace(workspace.AgentID)
		if taskID == "" || agentID == "" {
			return Plan{}, errors.New("task workspace task_id and agent_id are required")
		}
		if seenTasks[taskID] {
			return Plan{}, fmt.Errorf("duplicate task workspace: %s", taskID)
		}
		seenTasks[taskID] = true
		if workspace.PreImage.IsZero() {
			return Plan{}, fmt.Errorf("task workspace %s has no pre-image", taskID)
		}
		digest := workspace.PreImage.Digest()
		if expectedDigest == "" {
			expectedDigest = digest
		} else if digest != expectedDigest {
			return Plan{}, fmt.Errorf("task workspace %s does not share the integration pre-image", taskID)
		}

		inspection, ok := inspections[taskID]
		if !ok {
			return Plan{}, fmt.Errorf("missing inspection for task workspace %s", taskID)
		}
		if len(inspection.Changes) == 0 {
			continue
		}
		if err := staging.EnsureChangesMatchDeclared(inspection.Changes, workspace.Declared); err != nil {
			return Plan{}, fmt.Errorf("inspection for %s violates declared scope: %w", taskID, err)
		}
		for _, change := range inspection.Changes {
			changes = append(changes, PlannedChange{
				TaskID:  taskID,
				AgentID: agentID,
				Path:    change.Path,
				Action:  change.Action,
			})
		}
	}

	for taskID := range inspections {
		if !seenTasks[taskID] {
			return Plan{}, fmt.Errorf("inspection references unknown task workspace %s", taskID)
		}
	}
	if len(changes) == 0 {
		return Plan{}, errors.New("integration plan contains no actual changes")
	}

	sort.Slice(changes, func(i, j int) bool {
		if changes[i].Path != changes[j].Path {
			return changes[i].Path < changes[j].Path
		}
		if changes[i].TaskID != changes[j].TaskID {
			return changes[i].TaskID < changes[j].TaskID
		}
		return changes[i].Action < changes[j].Action
	})
	for i := 1; i < len(changes); i++ {
		if changes[i-1].Path == changes[i].Path {
			return Plan{}, fmt.Errorf(
				"integration conflict on %s between tasks %s and %s",
				changes[i].Path,
				changes[i-1].TaskID,
				changes[i].TaskID,
			)
		}
	}

	return Plan{
		PreImageDigest: expectedDigest,
		Changes:        changes,
	}, nil
}
