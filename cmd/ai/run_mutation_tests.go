package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/artifact"
	"github.com/FernandoVillegas27/ai-engineering-os/core/collaboration"
	"github.com/FernandoVillegas27/ai-engineering-os/core/controlplane"
	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/integration"
	"github.com/FernandoVillegas27/ai-engineering-os/core/message"
	"github.com/FernandoVillegas27/ai-engineering-os/core/mutation"
	"github.com/FernandoVillegas27/ai-engineering-os/core/proposal"
	"github.com/FernandoVillegas27/ai-engineering-os/core/scheduler"
	"github.com/FernandoVillegas27/ai-engineering-os/core/staging"
)

type mutationTestsResult struct {
	Workspace integration.IntegratedWorkspace
	Staged    mutation.StagedSet
	Applied   bool
}

func executeMutationTestsPhase(
	ctx context.Context,
	options runExecuteOptions,
	runID string,
	base integration.IntegratedWorkspace,
	testsOnly []execution.WorkUnit,
	artifactStore *artifact.Store,
	messageBus *message.Bus,
	eventStore *controlplane.EventStore,
	routing modelRoutingOptions,
	integrationParent string,
) (mutationTestsResult, error) {
	if len(testsOnly) == 0 {
		return mutationTestsResult{Workspace: base}, nil
	}

	handoff, err := artifact.New(artifact.NewRequest{
		RunID:         runID,
		TaskID:        "source-integration",
		ProducerAgent: "orchestrator",
		Type:          artifact.TypeHandoff,
		Summary: fmt.Sprintf(
			"Integrated source implementation is ready for tests-only QA (%d changed file(s)).",
			len(base.Changes),
		),
		Payload: map[string]any{
			"phase":   "source-integration",
			"changes": base.Changes,
		},
	}, time.Now().UTC())
	if err != nil {
		return mutationTestsResult{}, err
	}
	if err := artifactStore.Save(handoff); err != nil {
		return mutationTestsResult{}, fmt.Errorf("persist source integration handoff: %w", err)
	}
	if err := appendCollaborationEvent(
		ctx,
		eventStore,
		runID,
		controlplane.EventArtifactCreated,
		"source-integration",
		"orchestrator",
		handoff.ID,
		[]string{handoff.ID},
		nil,
	); err != nil {
		return mutationTestsResult{}, err
	}

	provider := collaboration.InboxContextProvider{
		RunID:         runID,
		MessageBus:    messageBus,
		ArtifactStore: artifactStore,
	}
	prepared := make([]execution.WorkUnit, len(testsOnly))
	for i, unit := range testsOnly {
		msg, err := message.New(message.NewRequest{
			RunID:     runID,
			TaskID:    unit.ID,
			FromAgent: "orchestrator",
			To: message.Target{
				Kind: message.TargetAgent,
				ID:   unit.AgentID,
			},
			Kind:         message.KindHandoff,
			Priority:     message.PriorityNormal,
			ArtifactRefs: []string{handoff.ID},
			Payload: map[string]any{
				"from_phase": "source-integration",
				"to_task":    unit.ID,
				"summary":    handoff.Summary,
			},
			Hop: 1,
		}, time.Now().UTC())
		if err != nil {
			return mutationTestsResult{}, err
		}
		persisted, err := messageBus.Append(ctx, msg)
		if err != nil {
			return mutationTestsResult{}, fmt.Errorf("persist tests-only handoff for %s: %w", unit.ID, err)
		}
		if err := appendCollaborationEvent(
			ctx,
			eventStore,
			runID,
			controlplane.EventAgentMessageSent,
			unit.ID,
			"orchestrator",
			persisted.ID,
			nil,
			[]string{persisted.ID},
		); err != nil {
			return mutationTestsResult{}, err
		}
		unit.ProjectRoot = base.Root
		contextValue, err := provider.Context(ctx, scheduler.Task{
			ID:      unit.ID,
			AgentID: unit.AgentID,
		})
		if err != nil {
			return mutationTestsResult{}, fmt.Errorf("load tests-only inbox for %s: %w", unit.ID, err)
		}
		unit.CollaborationContext = contextValue
		prepared[i] = unit

		if err := appendSimpleRunEvent(
			ctx,
			eventStore,
			runID,
			controlplane.EventTaskStarted,
			unit.ID,
			unit.AgentID,
			"tests-only QA proposal/stage started",
			"tests-only-qa",
		); err != nil {
			return mutationTestsResult{}, err
		}
	}

	frozen, err := mutation.FreezeProposals(ctx, prepared, func(unit execution.WorkUnit) (proposal.Runtime, error) {
		return routedProposalRuntime{runtimeName: options.runtime, routing: routing}, nil
	}, mutation.FreezeConfig{
		MaxConcurrency: options.maxConcurrency,
	})
	if err != nil {
		return mutationTestsResult{}, fmt.Errorf("freeze tests-only proposals: %w", err)
	}
	if err := mutation.ValidateFrozenTestsOnly(frozen); err != nil {
		return mutationTestsResult{}, err
	}

	staged, err := mutation.StageFrozenProposals(
		ctx,
		prepared,
		frozen,
		func(unit execution.WorkUnit) (staging.Runtime, error) {
			runtime, _, runtimeErr := stageRuntimeFor(options.runtime, routing, false)
			return runtime, runtimeErr
		},
		mutation.StageConfig{MaxConcurrency: options.maxConcurrency},
	)
	if err != nil {
		return mutationTestsResult{Staged: staged}, fmt.Errorf("stage tests-only proposals: %w", err)
	}

	final, err := integration.OverlayTestWorkspaces(
		ctx,
		integration.TestOverlayConfig{Parent: integrationParent},
		base,
		staged.Workspaces,
		staged.Inspections,
	)
	if err != nil {
		return mutationTestsResult{Staged: staged}, fmt.Errorf("overlay tests-only workspaces: %w", err)
	}

	for _, workspace := range staged.Workspaces {
		if err := appendSimpleRunEvent(
			ctx,
			eventStore,
			runID,
			controlplane.EventTaskCompleted,
			workspace.TaskID,
			workspace.AgentID,
			"tests-only QA staging completed",
			"tests-only-qa",
		); err != nil {
			_ = integration.Cleanup(final, integrationParent)
			return mutationTestsResult{Staged: staged}, err
		}
	}

	return mutationTestsResult{
		Workspace: final,
		Staged:    staged,
		Applied:   true,
	}, nil
}

func cleanupTestsOnlyStages(set mutation.StagedSet) {
	for _, workspace := range set.Workspaces {
		if strings.TrimSpace(workspace.Root) != "" {
			_ = os.RemoveAll(workspace.Root)
		}
	}
}

func appendSimpleRunEvent(
	ctx context.Context,
	store *controlplane.EventStore,
	runID string,
	kind controlplane.EventType,
	taskID, agentID, summary, phase string,
) error {
	event, err := controlplane.NewEvent(controlplane.NewEventRequest{
		RunID:   runID,
		Type:    kind,
		TaskID:  taskID,
		AgentID: agentID,
		Summary: summary,
		Data: map[string]any{
			"phase": phase,
		},
	}, time.Now().UTC())
	if err != nil {
		return err
	}
	_, err = store.Append(ctx, event)
	return err
}

func appendCollaborationEvent(
	ctx context.Context,
	store *controlplane.EventStore,
	runID string,
	kind controlplane.EventType,
	taskID, agentID, summary string,
	artifactRefs, messageRefs []string,
) error {
	event, err := controlplane.NewEvent(controlplane.NewEventRequest{
		RunID:        runID,
		Type:         kind,
		TaskID:       taskID,
		AgentID:      agentID,
		Summary:      summary,
		ArtifactRefs: artifactRefs,
		MessageRefs:  messageRefs,
		Data: map[string]any{
			"phase": "tests-only-qa",
		},
	}, time.Now().UTC())
	if err != nil {
		return err
	}
	_, err = store.Append(ctx, event)
	return err
}
