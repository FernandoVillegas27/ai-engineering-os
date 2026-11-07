package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/artifact"
	"github.com/FernandoVillegas27/ai-engineering-os/core/controlplane"
	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/integration"
	"github.com/FernandoVillegas27/ai-engineering-os/core/message"
	"github.com/FernandoVillegas27/ai-engineering-os/core/modelpolicy"
	"github.com/FernandoVillegas27/ai-engineering-os/core/orchestration"
	"github.com/FernandoVillegas27/ai-engineering-os/core/promotion"
	"github.com/FernandoVillegas27/ai-engineering-os/core/promotioncandidate"
	"github.com/FernandoVillegas27/ai-engineering-os/core/runcheckpoint"
	"github.com/FernandoVillegas27/ai-engineering-os/core/runstate"
	"github.com/FernandoVillegas27/ai-engineering-os/core/staging"
	"github.com/FernandoVillegas27/ai-engineering-os/integrations/localverify"
)

func runRunResumeMutation(
	options runResumeOptions,
	run runstate.Run,
	runStore *runstate.Store,
) error {
	if !run.ProjectCodeTrusted {
		return fmt.Errorf("mutating run %s lacks explicit project-code trust evidence", run.ID)
	}
	if strings.TrimSpace(run.ProjectRoot) == "" {
		return fmt.Errorf("mutating run %s has no durable project root", run.ID)
	}
	eventStore, err := openControlPlaneStore(options.eventsDir)
	if err != nil {
		return err
	}
	events, err := eventStore.List(run.ID)
	if err != nil {
		return err
	}
	if len(events) == 0 || events[0].Type != controlplane.EventRunStarted {
		return fmt.Errorf("run %s has no valid Control Plane event stream", run.ID)
	}
	state, err := controlplane.Project(events)
	if err != nil {
		return fmt.Errorf("project paused mutation run: %w", err)
	}
	if state.Status != controlplane.RunPaused {
		return fmt.Errorf("Control Plane run %s is not paused: status=%s", run.ID, state.Status)
	}

	phase := runcheckpoint.Phase(state.Pipeline.CheckpointPhase)
	switch phase {
	case runcheckpoint.PhaseSourceIntegrated, runcheckpoint.PhaseFinalIntegrated:
	default:
		return fmt.Errorf(
			"mutating run %s has no resumable integration checkpoint: %q",
			run.ID,
			state.Pipeline.CheckpointPhase,
		)
	}

	checkpointRoot, err := resolveRunAuxDir(
		options.checkpointDir,
		"AI_ENGINEERING_OS_CHECKPOINT_DIR",
		"run-checkpoints",
	)
	if err != nil {
		return err
	}
	checkpointStore, err := runcheckpoint.NewStore(runcheckpoint.Config{Root: checkpointRoot})
	if err != nil {
		return err
	}
	integrationParent, err := resolveMutationIntegrationParent(run.ProjectRoot)
	if err != nil {
		return err
	}
	restored, err := checkpointStore.Restore(
		context.Background(),
		run.ID,
		phase,
		integrationParent,
	)
	if err != nil {
		return fmt.Errorf("restore mutation checkpoint %s: %w", phase, err)
	}
	finalIntegrated := restored
	keepIntegrated := false
	defer func() {
		if finalIntegrated.Root != "" && finalIntegrated.Root != restored.Root && !keepIntegrated {
			_ = integration.Cleanup(finalIntegrated, integrationParent)
		}
		if restored.Root != "" && (restored.Root != finalIntegrated.Root || !keepIntegrated) {
			_ = integration.Cleanup(restored, integrationParent)
		}
	}()

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

	policy := modelpolicy.PolicyAuto
	if strings.TrimSpace(run.Runtime.ModelPolicy) != "" {
		policy, err = modelpolicy.Parse(run.Runtime.ModelPolicy)
		if err != nil {
			return fmt.Errorf("parse durable model policy: %w", err)
		}
	}
	routing := modelRoutingOptions{
		Policy:   policy,
		Override: run.Runtime.Model,
	}
	switch run.Runtime.Provider {
	case "auto", "codex", "cursor":
	default:
		return fmt.Errorf("mutation resume does not support durable runtime %q", run.Runtime.Provider)
	}
	resumeOptions := runExecuteOptions{
		runtime:        run.Runtime.Provider,
		modelPolicy:    policy,
		model:          run.Runtime.Model,
		maxConcurrency: options.maxConcurrency,
	}

	controlStore, err := openRunControlStore(options.controlDir)
	if err != nil {
		return err
	}
	active, err := startActiveRunContext(context.Background(), controlStore, run.ID)
	if err != nil {
		return fmt.Errorf("acquire run lease before mutation resume: %w", err)
	}
	defer active.Stop()

	if _, err := controlStore.PauseRequested(run.ID); err != nil {
		return err
	}
	if err := controlStore.ClearPause(run.ID); err != nil {
		return err
	}

	now := time.Now().UTC()
	if now.Before(run.UpdatedAt) {
		now = run.UpdatedAt
	}
	if err := runstate.Transition(&run, runstate.StatusRunning, now); err != nil {
		return err
	}
	if err := runStore.Save(run); err != nil {
		return err
	}
	if err := appendSimpleRunEvent(
		context.Background(),
		eventStore,
		run.ID,
		controlplane.EventRunResumed,
		"", "",
		fmt.Sprintf("coordinated mutating run resumed from %s checkpoint", phase),
		string(phase),
	); err != nil {
		rollbackAt := time.Now().UTC()
		if rollbackAt.Before(run.UpdatedAt) {
			rollbackAt = run.UpdatedAt
		}
		if transitionErr := runstate.Transition(&run, runstate.StatusPaused, rollbackAt); transitionErr == nil {
			_ = runStore.Save(run)
		}
		return err
	}

	ctx := active.Context()

	if phase == runcheckpoint.PhaseSourceIntegrated {
		testsOnly, err := reconstructMutationUnits(run, "tests-only")
		if err != nil {
			_ = abortMutationRun(ctx, runStore, eventStore, &run)
			return err
		}
		qaResult, err := executeMutationTestsPhase(
			ctx,
			resumeOptions,
			run.ID,
			restored,
			testsOnly,
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
		if _, err := checkpointStore.Save(
			ctx,
			run.ID,
			runcheckpoint.PhaseFinalIntegrated,
			run.ProjectRoot,
			finalIntegrated,
			time.Now().UTC(),
		); err != nil {
			_ = abortMutationRun(ctx, runStore, eventStore, &run)
			return fmt.Errorf("persist resumed final integration checkpoint: %w", err)
		}
		if err := appendSimpleRunEvent(
			ctx,
			eventStore,
			run.ID,
			controlplane.EventCheckpointCreated,
			"", "",
			"final integration checkpoint persisted after resume",
			string(runcheckpoint.PhaseFinalIntegrated),
		); err != nil {
			_ = abortMutationRun(ctx, runStore, eventStore, &run)
			return err
		}
		paused, err := pauseMutationAtCheckpoint(
			ctx,
			&run,
			runStore,
			eventStore,
			controlStore,
			runcheckpoint.PhaseFinalIntegrated,
		)
		if err != nil {
			_ = abortMutationRun(ctx, runStore, eventStore, &run)
			return err
		}
		if paused {
			return renderMutationResumePaused(options, run, runcheckpoint.PhaseFinalIntegrated)
		}
	} else {
		if err := persistMutationCheckpointState(
			&run,
			runStore,
			runcheckpoint.PhaseFinalIntegrated,
		); err != nil {
			_ = abortMutationRun(ctx, runStore, eventStore, &run)
			return err
		}
	}

	if err := appendSimpleRunEvent(
		ctx,
		eventStore,
		run.ID,
		controlplane.EventVerificationStarted,
		"", "",
		"run-wide integration quality resumed",
		"quality",
	); err != nil {
		_ = abortMutationRun(ctx, runStore, eventStore, &run)
		return err
	}

	reviewer, _, err := reviewRuntimeFor(run.Runtime.Provider, routing, false)
	if err != nil {
		_ = abortMutationRun(ctx, runStore, eventStore, &run)
		return err
	}
	corrector, _, err := correctorFor(run.Runtime.Provider, routing, false)
	if err != nil {
		_ = abortMutationRun(ctx, runStore, eventStore, &run)
		return err
	}

	checkpointPlan := integration.Plan{
		PreImageDigest: finalIntegrated.PreImage.Digest(),
	}
	gate, qualityErr := integration.RunQualityGate(ctx, integration.QualityGateConfig{
		OriginalRoot:    run.ProjectRoot,
		Plan:            checkpointPlan,
		Workspace:       finalIntegrated,
		ExpectedChanges: finalIntegrated.Changes,
		Task:            run.Task,
		Verifier:        localverify.New(),
		Reviewer:        reviewer,
		Corrector:       corrector,
		MaxAttempts:     3,
		TempParent:      integrationParent,
	})
	if qualityErr != nil {
		_ = appendSimpleRunEvent(
			context.WithoutCancel(ctx),
			eventStore,
			run.ID,
			controlplane.EventVerificationFailed,
			"", "",
			qualityErr.Error(),
			"quality",
		)
		_ = abortMutationRun(ctx, runStore, eventStore, &run)
		return fmt.Errorf("resumed run-wide integration quality: %w", qualityErr)
	}
	if !gate.Promotable {
		_ = appendSimpleRunEvent(
			ctx,
			eventStore,
			run.ID,
			controlplane.EventVerificationFailed,
			"", "",
			string(gate.Quality.Reason),
			"quality",
		)
		_ = abortMutationRun(ctx, runStore, eventStore, &run)
		return fmt.Errorf("resumed integrated result is not promotable: %s", gate.Quality.Reason)
	}
	if err := appendSimpleRunEvent(
		ctx,
		eventStore,
		run.ID,
		controlplane.EventVerificationPassed,
		"", "",
		"resumed run-wide integration quality passed",
		"quality",
	); err != nil {
		_ = abortMutationRun(ctx, runStore, eventStore, &run)
		return err
	}
	if err := appendSimpleRunEvent(
		ctx,
		eventStore,
		run.ID,
		controlplane.EventReviewApproved,
		"", "",
		"resumed integrated result approved by independent review",
		"quality",
	); err != nil {
		_ = abortMutationRun(ctx, runStore, eventStore, &run)
		return err
	}

	candidate, err := integration.BuildPromotionCandidate(
		run.ProjectRoot,
		finalIntegrated,
		gate,
		integrationParent,
	)
	if err != nil {
		_ = abortMutationRun(ctx, runStore, eventStore, &run)
		return err
	}
	promotionPlan, err := promotion.DryRun(ctx, candidate.Request)
	if err != nil {
		_ = abortMutationRun(ctx, runStore, eventStore, &run)
		return fmt.Errorf("resumed promotion dry-run: %w", err)
	}
	if !promotionPlan.Ready {
		_ = abortMutationRun(ctx, runStore, eventStore, &run)
		return fmt.Errorf("resumed promotion dry-run did not produce a ready plan")
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
		return fmt.Errorf("persist resumed promotion candidate: %w", err)
	}

	if err := appendSimpleRunEvent(
		ctx,
		eventStore,
		run.ID,
		controlplane.EventApprovalRequested,
		"", "",
		"explicit promotion approval required after resume",
		"promotion",
	); err != nil {
		_ = abortMutationRun(ctx, runStore, eventStore, &run)
		return err
	}

	for index := range run.WorkUnits {
		run.WorkUnits[index].Status = runstate.WorkUnitCompleted
	}
	completeAt := time.Now().UTC()
	if completeAt.Before(run.UpdatedAt) {
		completeAt = run.UpdatedAt
	}
	if err := runstate.Transition(&run, runstate.StatusCompleted, completeAt); err != nil {
		return err
	}
	if err := runStore.Save(run); err != nil {
		return err
	}
	if err := appendSimpleRunEvent(
		ctx,
		eventStore,
		run.ID,
		controlplane.EventRunCompleted,
		"", "",
		"coordinated mutating run completed after checkpoint resume",
		"promotion",
	); err != nil {
		return err
	}

	keepIntegrated = true
	if options.json {
		return printJSON(struct {
			RunID              string                    `json:"run_id"`
			Status             runstate.Status           `json:"status"`
			RestoredFrom       runcheckpoint.Phase       `json:"restored_from"`
			IntegratedWorkspace string                   `json:"integrated_workspace"`
			PromotionReady     bool                      `json:"promotion_ready"`
			Changes            []staging.WorkspaceChange `json:"changes"`
		}{
			RunID:               run.ID,
			Status:              run.Status,
			RestoredFrom:        phase,
			IntegratedWorkspace: finalIntegrated.Root,
			PromotionReady:      promotionPlan.Ready,
			Changes:             gate.FinalChanges,
		})
	}

	fmt.Println("Resumed Coordinated Mutating Run")
	fmt.Printf("Run ID: %s\n", run.ID)
	fmt.Printf("Restored from: %s\n", phase)
	fmt.Printf("Status: %s\n", run.Status)
	fmt.Printf("Promotion candidate: READY (%d change(s))\n", len(gate.FinalChanges))
	fmt.Println("Approval required: YES")
	fmt.Println("Original project was NOT modified.")
	return nil
}

func reconstructMutationUnits(run runstate.Run, kind string) ([]execution.WorkUnit, error) {
	var out []execution.WorkUnit
	for _, durable := range run.WorkUnits {
		if !durable.Mutating {
			continue
		}
		if durable.MutationKind == "" {
			return nil, fmt.Errorf("mutating work unit %s has no durable mutation_kind", durable.ID)
		}
		if durable.MutationKind != kind {
			continue
		}
		mutation := execution.MutationCapability{}
		switch kind {
		case "source":
			mutation.SourceCode = true
			mutation.Tests = true
		case "tests-only":
			mutation.Tests = true
		default:
			return nil, fmt.Errorf("unsupported mutation kind: %s", kind)
		}
		out = append(out, execution.WorkUnit{
			ID:           durable.ID,
			Task:         durable.Task,
			ProjectRoot:  run.ProjectRoot,
			Intent:       orchestration.Intent(run.Intent),
			AgentID:      durable.AgentID,
			Capabilities: append([]string(nil), durable.Capabilities...),
			Skills:         append([]string(nil), durable.Skills...),
			Tools:          append([]string(nil), durable.Tools...),
			ToolOperations: execution.CloneToolOperations(durable.ToolOperations),
			Permissions:    durable.Permissions,
			Mutation:     mutation,
			Mode:         execution.ModeReadOnly,
		})
	}
	return out, nil
}

func renderMutationResumePaused(
	options runResumeOptions,
	run runstate.Run,
	phase runcheckpoint.Phase,
) error {
	if options.json {
		return printJSON(struct {
			RunID string              `json:"run_id"`
			Status runstate.Status     `json:"status"`
			Phase  runcheckpoint.Phase `json:"checkpoint_phase"`
		}{
			RunID: run.ID,
			Status: run.Status,
			Phase:  phase,
		})
	}
	fmt.Println("Resumed Mutating Run Paused Again")
	fmt.Printf("Run ID: %s\n", run.ID)
	fmt.Printf("Checkpoint: %s\n", phase)
	fmt.Printf("Resume: ai run resume %s\n", run.ID)
	return nil
}
