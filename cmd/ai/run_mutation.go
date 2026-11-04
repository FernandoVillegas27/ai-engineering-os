package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/artifact"
	"github.com/FernandoVillegas27/ai-engineering-os/core/controlplane"
	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/integration"
	"github.com/FernandoVillegas27/ai-engineering-os/core/message"
	"github.com/FernandoVillegas27/ai-engineering-os/core/mutation"
	"github.com/FernandoVillegas27/ai-engineering-os/core/promotion"
	"github.com/FernandoVillegas27/ai-engineering-os/core/promotioncandidate"
	"github.com/FernandoVillegas27/ai-engineering-os/core/proposal"
	"github.com/FernandoVillegas27/ai-engineering-os/core/runcontrol"
	"github.com/FernandoVillegas27/ai-engineering-os/core/runstate"
	"github.com/FernandoVillegas27/ai-engineering-os/core/staging"
	"github.com/FernandoVillegas27/ai-engineering-os/core/taskgraph"
	"github.com/FernandoVillegas27/ai-engineering-os/integrations/localverify"
)

type routedProposalRuntime struct {
	runtimeName string
	routing     modelRoutingOptions
}

func (r routedProposalRuntime) ID() string { return "coordinated-proposal-routing" }

func (r routedProposalRuntime) Propose(ctx context.Context, unit execution.WorkUnit) (proposal.Proposal, error) {
	generated, err := generateProposalWithRouting(ctx, unit, r.runtimeName, r.routing)
	if err != nil {
		return proposal.Proposal{}, err
	}
	return generated.Proposal, nil
}

func runCoordinatedMutation(
	ctx context.Context,
	options runExecuteOptions,
	root string,
	capabilities []string,
	intent string,
	units []execution.WorkUnit,
) error {
	if !options.multiAgentWrite {
		return fmt.Errorf("coordinated mutation requires explicit --multi-agent-write")
	}
	if options.dryRun {
		return fmt.Errorf("--multi-agent-write cannot be combined with --dry-run")
	}

	mutatingUnits, err := mutation.SelectSourceMutatingUnits(units)
	if err != nil {
		return err
	}
	testsOnlyUnits := mutation.SelectTestsOnlyUnits(units)
	fullTasks, err := taskgraph.Build(units)
	if err != nil {
		return err
	}
	preStagePlan, err := mutation.BuildPreStagePlan(units, fullTasks, mutatingUnits)
	if err != nil {
		return err
	}
	participants := mutationParticipants(preStagePlan.Units, mutatingUnits, testsOnlyUnits)

	runStore, err := openRunStore(options.stateDir)
	if err != nil {
		return err
	}
	eventStore, err := openControlPlaneStore(options.eventsDir)
	if err != nil {
		return err
	}
	artifactsRoot, err := resolveRunAuxDir(options.artifactsDir, "AI_ENGINEERING_OS_ARTIFACTS_DIR", "artifacts")
	if err != nil {
		return err
	}
	artifactStore, err := artifact.NewStore(artifactsRoot)
	if err != nil {
		return err
	}
	messagesRoot, err := resolveRunAuxDir(options.messagesDir, "AI_ENGINEERING_OS_MESSAGES_DIR", "messages")
	if err != nil {
		return err
	}
	messageBus, err := message.NewBus(messagesRoot)
	if err != nil {
		return err
	}

	run, err := runstate.New(runstate.NewRunRequest{
		Task:         options.task,
		ProjectID:    options.projectID,
		ProjectRoot:  root,
		Capabilities: capabilities,
		Runtime: runstate.RuntimeSelection{
			Provider: options.runtime,
			Model:    options.model,
		},
		WorkUnits: participants,
	}, time.Now().UTC())
	if err != nil {
		return err
	}
	if err := runStore.Save(run); err != nil {
		return err
	}
	if err := runstate.Transition(&run, runstate.StatusRunning, time.Now().UTC()); err != nil {
		return err
	}
	if err := runStore.Save(run); err != nil {
		return err
	}

	controlStore, err := openRunControlStore(options.controlDir)
	if err != nil {
		_ = abortMutationRun(ctx, runStore, eventStore, &run)
		return err
	}
	runCtx, stopWatcher, err := runcontrol.WithCancellationWatcher(ctx, controlStore, run.ID, 250*time.Millisecond)
	if err != nil {
		_ = abortMutationRun(ctx, runStore, eventStore, &run)
		return err
	}
	defer stopWatcher()
	ctx = runCtx

	emit := func(kind controlplane.EventType, taskID, agentID, summary string) error {
		event, eventErr := controlplane.NewEvent(controlplane.NewEventRequest{
			RunID:   run.ID,
			Type:    kind,
			TaskID:  taskID,
			AgentID: agentID,
			Summary: summary,
			Data: map[string]any{
				"mode": "coordinated-mutation",
			},
		}, time.Now().UTC())
		if eventErr != nil {
			return eventErr
		}
		_, eventErr = eventStore.Append(ctx, event)
		return eventErr
	}

	if err := emit(controlplane.EventRunStarted, "", "", "coordinated mutating run started"); err != nil {
		return err
	}
	for _, unit := range participants {
		summary := "source-mutating work unit created"
		switch {
		case unit.Mutation.Tests && !unit.Mutation.SourceCode:
			summary = "tests-only post-integration work unit created"
		case !unit.Mutation.SourceCode:
			summary = "read-only pre-stage work unit created"
		}
		if err := emit(controlplane.EventTaskCreated, unit.ID, "", summary); err != nil {
			return err
		}
		if err := emit(controlplane.EventTaskAssigned, unit.ID, unit.AgentID, "mutating work unit assigned"); err != nil {
			return err
		}
	}

	routing := modelRoutingOptions{Policy: options.modelPolicy, Override: options.model}
	mutatingUnits, err = executeMutationPreStage(
		ctx,
		options,
		run.ID,
		preStagePlan,
		mutatingUnits,
		artifactStore,
		messageBus,
		eventStore,
		routing,
	)
	if err != nil {
		_ = abortMutationRun(ctx, runStore, eventStore, &run)
		return err
	}

	if err := appendSimpleRunEvent(ctx, eventStore, run.ID, controlplane.EventProposalStarted, "", "", "source proposal freeze started", "source"); err != nil {
		_ = abortMutationRun(ctx, runStore, eventStore, &run)
		return err
	}
	frozen, err := mutation.FreezeProposals(ctx, mutatingUnits, func(unit execution.WorkUnit) (proposal.Runtime, error) {
		// Routed runtime preserves the existing proposal alignment/escalation behavior.
		return routedProposalRuntime{runtimeName: options.runtime, routing: routing}, nil
	}, mutation.FreezeConfig{
		MaxConcurrency: options.maxConcurrency,
	})
	if err != nil {
		_ = appendSimpleRunEvent(ctx, eventStore, run.ID, controlplane.EventProposalFailed, "", "", err.Error(), "source")
		_ = abortMutationRun(ctx, runStore, eventStore, &run)
		return fmt.Errorf("freeze mutating proposals: %w", err)
	}
	if err := appendSimpleRunEvent(ctx, eventStore, run.ID, controlplane.EventProposalCompleted, "", "", "source proposal freeze completed", "source"); err != nil {
		_ = abortMutationRun(ctx, runStore, eventStore, &run)
		return err
	}
	for _, item := range frozen {
		if err := emit(controlplane.EventArtifactCreated, item.TaskID, item.AgentID, "proposal ownership frozen"); err != nil {
			_ = abortMutationRun(ctx, runStore, eventStore, &run)
			return err
		}
	}

	if err := appendSimpleRunEvent(ctx, eventStore, run.ID, controlplane.EventStageStarted, "", "", "source isolated staging started", "source"); err != nil {
		_ = abortMutationRun(ctx, runStore, eventStore, &run)
		return err
	}
	staged, err := mutation.StageFrozenProposals(ctx, mutatingUnits, frozen, func(unit execution.WorkUnit) (staging.Runtime, error) {
		runtime, _, runtimeErr := stageRuntimeFor(options.runtime, routing, false)
		return runtime, runtimeErr
	}, mutation.StageConfig{
		MaxConcurrency: options.maxConcurrency,
	})
	defer cleanupMutationStages(staged)
	if err != nil {
		_ = appendSimpleRunEvent(ctx, eventStore, run.ID, controlplane.EventStageFailed, "", "", err.Error(), "source")
		_ = abortMutationRun(ctx, runStore, eventStore, &run)
		return fmt.Errorf("stage frozen proposals: %w", err)
	}
	if err := appendSimpleRunEvent(ctx, eventStore, run.ID, controlplane.EventStageCompleted, "", "", "source isolated staging completed", "source"); err != nil {
		_ = abortMutationRun(ctx, runStore, eventStore, &run)
		return err
	}
	for _, workspace := range staged.Workspaces {
		if err := emit(controlplane.EventTaskCompleted, workspace.TaskID, workspace.AgentID, "isolated task staging completed"); err != nil {
			_ = abortMutationRun(ctx, runStore, eventStore, &run)
			return err
		}
	}

	if err := appendSimpleRunEvent(ctx, eventStore, run.ID, controlplane.EventIntegrationStarted, "", "", "source integration started", "source"); err != nil {
		_ = abortMutationRun(ctx, runStore, eventStore, &run)
		return err
	}
	integrationPlan, err := integration.Build(staged.Workspaces, staged.Inspections)
	if err != nil {
		_ = appendSimpleRunEvent(ctx, eventStore, run.ID, controlplane.EventIntegrationFailed, "", "", err.Error(), "source")
		_ = abortMutationRun(ctx, runStore, eventStore, &run)
		return fmt.Errorf("build integration plan: %w", err)
	}

	integrationParent, err := resolveMutationIntegrationParent(root)
	if err != nil {
		_ = abortMutationRun(ctx, runStore, eventStore, &run)
		return err
	}
	sourceIntegrated, err := integration.Materialize(ctx, integration.MaterializeConfig{
		OriginalRoot: root,
		Parent:       integrationParent,
	}, integrationPlan, staged.Workspaces)
	if err != nil {
		_ = appendSimpleRunEvent(ctx, eventStore, run.ID, controlplane.EventIntegrationFailed, "", "", err.Error(), "source")
		_ = abortMutationRun(ctx, runStore, eventStore, &run)
		return fmt.Errorf("materialize integrated workspace: %w", err)
	}
	if err := appendSimpleRunEvent(ctx, eventStore, run.ID, controlplane.EventIntegrationCompleted, "", "", "source integration completed", "source"); err != nil {
		_ = abortMutationRun(ctx, runStore, eventStore, &run)
		return err
	}

	finalIntegrated := sourceIntegrated
	keepIntegrated := false
	defer func() {
		if finalIntegrated.Root != "" && finalIntegrated.Root != sourceIntegrated.Root && !keepIntegrated {
			_ = integration.Cleanup(finalIntegrated, integrationParent)
		}
		if sourceIntegrated.Root != "" && (sourceIntegrated.Root != finalIntegrated.Root || !keepIntegrated) {
			_ = integration.Cleanup(sourceIntegrated, integrationParent)
		}
	}()

	qaResult, err := executeMutationTestsPhase(
		ctx,
		options,
		run.ID,
		sourceIntegrated,
		testsOnlyUnits,
		artifactStore,
		messageBus,
		eventStore,
		routing,
		integrationParent,
	)
	defer cleanupTestsOnlyStages(qaResult.Staged)
	if err != nil {
		_ = abortMutationRun(ctx, runStore, eventStore, &run)
		return err
	}
	if qaResult.Applied {
		finalIntegrated = qaResult.Workspace
	}

	if err := emit(controlplane.EventVerificationStarted, "", "", "run-wide integration quality started"); err != nil {
		_ = abortMutationRun(ctx, runStore, eventStore, &run)
		return err
	}

	reviewer, _, err := reviewRuntimeFor(options.runtime, routing, false)
	if err != nil {
		_ = abortMutationRun(ctx, runStore, eventStore, &run)
		return err
	}
	corrector, _, err := correctorFor(options.runtime, routing, false)
	if err != nil {
		_ = abortMutationRun(ctx, runStore, eventStore, &run)
		return err
	}

	gate, qualityErr := integration.RunQualityGate(ctx, integration.QualityGateConfig{
		OriginalRoot: root,
		Plan:         integrationPlan,
		Workspace:       finalIntegrated,
		ExpectedChanges: finalIntegrated.Changes,
		Task:         options.task,
		Verifier:     localverify.New(),
		Reviewer:     reviewer,
		Corrector:    corrector,
		MaxAttempts:  3,
		TempParent:   integrationParent,
	})
	if qualityErr != nil {
		_ = emit(controlplane.EventVerificationFailed, "", "", qualityErr.Error())
		_ = abortMutationRun(ctx, runStore, eventStore, &run)
		return fmt.Errorf("run-wide integration quality: %w", qualityErr)
	}
	if !gate.Promotable {
		_ = emit(controlplane.EventVerificationFailed, "", "", string(gate.Quality.Reason))
		_ = abortMutationRun(ctx, runStore, eventStore, &run)
		return fmt.Errorf("integrated result is not promotable: %s", gate.Quality.Reason)
	}
	if err := emit(controlplane.EventVerificationPassed, "", "", "run-wide integration quality passed"); err != nil {
		_ = abortMutationRun(ctx, runStore, eventStore, &run)
		return err
	}
	if err := emit(controlplane.EventReviewApproved, "", "", "integrated result approved by independent review"); err != nil {
		_ = abortMutationRun(ctx, runStore, eventStore, &run)
		return err
	}

	candidate, err := integration.BuildPromotionCandidate(root, finalIntegrated, gate, integrationParent)
	if err != nil {
		_ = abortMutationRun(ctx, runStore, eventStore, &run)
		return err
	}
	promotionPlan, err := promotion.DryRun(ctx, candidate.Request)
	if err != nil {
		_ = abortMutationRun(ctx, runStore, eventStore, &run)
		return fmt.Errorf("promotion dry-run: %w", err)
	}
	if !promotionPlan.Ready {
		_ = abortMutationRun(ctx, runStore, eventStore, &run)
		return fmt.Errorf("promotion dry-run did not produce a ready plan")
	}

	candidateRoot, err := resolveRunAuxDir(
		options.promotionDir,
		"AI_ENGINEERING_OS_PROMOTION_DIR",
		"promotion-candidates",
	)
	if err != nil {
		_ = abortMutationRun(ctx, runStore, eventStore, &run)
		return err
	}
	candidateStore, err := promotioncandidate.NewStore(candidateRoot)
	if err != nil {
		_ = abortMutationRun(ctx, runStore, eventStore, &run)
		return err
	}
	record, err := promotioncandidate.New(run.ID, candidate.Request, time.Now().UTC())
	if err != nil {
		_ = abortMutationRun(ctx, runStore, eventStore, &run)
		return err
	}
	if err := candidateStore.Save(record); err != nil {
		_ = abortMutationRun(ctx, runStore, eventStore, &run)
		return fmt.Errorf("persist promotion candidate: %w", err)
	}

	if err := emit(controlplane.EventApprovalRequested, "", "", "explicit promotion approval required"); err != nil {
		_ = abortMutationRun(ctx, runStore, eventStore, &run)
		return err
	}

	for index := range run.WorkUnits {
		run.WorkUnits[index].Status = runstate.WorkUnitCompleted
	}
	if err := runstate.Transition(&run, runstate.StatusCompleted, time.Now().UTC()); err != nil {
		return err
	}
	if err := runStore.Save(run); err != nil {
		return err
	}
	if err := emit(controlplane.EventRunCompleted, "", "", "coordinated mutating run completed with promotion candidate ready"); err != nil {
		return err
	}

	// Preserve only the final integrated workspace. It is still outside the
	// original project and promotion.Apply has NOT been called.
	keepIntegrated = true

	if options.json {
		return printJSON(struct {
			RunID              string                    `json:"run_id"`
			Intent             string                    `json:"intent"`
			IntegratedWorkspace string                   `json:"integrated_workspace"`
			QualityPassed       bool                     `json:"quality_passed"`
			Promotable          bool                     `json:"promotable"`
			PromotionReady      bool                     `json:"promotion_ready"`
			Changes             []staging.WorkspaceChange `json:"changes"`
		}{
			RunID:               run.ID,
			Intent:              intent,
			IntegratedWorkspace: finalIntegrated.Root,
			QualityPassed:       gate.Quality.Passed,
			Promotable:          gate.Promotable,
			PromotionReady:      promotionPlan.Ready,
			Changes:             gate.FinalChanges,
		})
	}

	fmt.Println("Coordinated Mutating Run")
	fmt.Printf("Run ID: %s\n", run.ID)
	fmt.Printf("Intent: %s\n", intent)
	fmt.Printf("Source agents staged: %d\n", len(staged.Workspaces))
	fmt.Printf("Tests-only agents staged: %d\n", len(qaResult.Staged.Workspaces))
	fmt.Printf("Integrated workspace: %s\n", finalIntegrated.Root)
	fmt.Printf("Quality: %s\n", gate.Quality.Reason)
	fmt.Printf("Promotion candidate: READY (%d change(s))\n", len(gate.FinalChanges))
	fmt.Println("Promotion candidate persisted: YES")
	fmt.Println("Approval required: YES")
	fmt.Println()
	fmt.Println("The original project was NOT modified.")
	fmt.Println("No commit or push was performed.")
	fmt.Printf("Inspect run: ai run control %s\n", run.ID)
	return nil
}

func cleanupMutationStages(set mutation.StagedSet) {
	for _, workspace := range set.Workspaces {
		if strings.TrimSpace(workspace.Root) != "" {
			_ = os.RemoveAll(workspace.Root)
		}
	}
}

func resolveMutationIntegrationParent(root string) (string, error) {
	parent := filepath.Dir(root)
	info, err := os.Stat(parent)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("integration parent is unavailable")
	}
	return parent, nil
}

func abortMutationRun(ctx context.Context, store *runstate.Store, eventStore *controlplane.EventStore, run *runstate.Run) error {
	if ctx != nil && ctx.Err() != nil {
		for index := range run.WorkUnits {
			if run.WorkUnits[index].Status != runstate.WorkUnitCompleted {
				run.WorkUnits[index].Status = runstate.WorkUnitCancelled
			}
		}
		if run.Status != runstate.StatusCancelled {
			if err := runstate.Transition(run, runstate.StatusCancelled, time.Now().UTC()); err != nil {
				return err
			}
		}
		if err := store.Save(*run); err != nil {
			return err
		}
		if eventStore != nil {
			event, err := controlplane.NewEvent(controlplane.NewEventRequest{
				RunID:   run.ID,
				Type:    controlplane.EventRunCancelled,
				Summary: "run cancelled by durable control request",
				Data:    map[string]any{"source": "run-control"},
			}, time.Now().UTC())
			if err != nil {
				return err
			}
			if _, err := eventStore.Append(context.Background(), event); err != nil {
				return err
			}
		}
		return nil
	}
	if err := failMutationRun(store, run); err != nil {
		return err
	}
	if eventStore != nil {
		event, err := controlplane.NewEvent(controlplane.NewEventRequest{
			RunID:   run.ID,
			Type:    controlplane.EventRunFailed,
			Summary: "coordinated mutating run failed",
			Data:    map[string]any{"source": "mutation-pipeline"},
		}, time.Now().UTC())
		if err != nil {
			return err
		}
		if _, err := eventStore.Append(context.Background(), event); err != nil {
			return err
		}
	}
	return nil
}

func failMutationRun(store *runstate.Store, run *runstate.Run) error {
	if store == nil || run == nil {
		return nil
	}
	for index := range run.WorkUnits {
		if run.WorkUnits[index].Status != runstate.WorkUnitCompleted {
			run.WorkUnits[index].Status = runstate.WorkUnitFailed
		}
	}
	if run.Status != runstate.StatusFailed {
		if err := runstate.Transition(run, runstate.StatusFailed, time.Now().UTC()); err != nil {
			return err
		}
	}
	return store.Save(*run)
}
