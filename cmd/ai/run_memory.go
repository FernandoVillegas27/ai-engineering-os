package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/FernandoVillegas27/ai-engineering-os/core/artifact"
	"github.com/FernandoVillegas27/ai-engineering-os/core/coordinator"
	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/memory"
	engramintegration "github.com/FernandoVillegas27/ai-engineering-os/integrations/engram"
)

type runMemoryReport struct {
	Recall   memory.RecallContext `json:"recall"`
	Artifact memory.PersistReport `json:"artifact_memory"`
	Learning memory.PersistReport `json:"run_learning"`
}

func runMemoryProvider(mode string) (memory.ProjectMemory, error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "auto", "engram":
		root, err := resolveMemoryGovernanceDir("")
		if err != nil {
			return nil, err
		}
		store, err := memory.NewGovernanceStore(root)
		if err != nil {
			return nil, err
		}
		client := engramintegration.New()
		return memory.GovernedProjectMemory{Store: store, Mirror: client}, nil
	case "off":
		return nil, nil
	default:
		return nil, fmt.Errorf("memory provider no soportado: %s", mode)
	}
}

func resolveMemoryGovernanceDir(explicit string) (string, error) {
	if strings.TrimSpace(explicit) != "" {
		return explicit, nil
	}
	if value := strings.TrimSpace(os.Getenv("AI_ENGINEERING_OS_MEMORY_DIR")); value != "" {
		return value, nil
	}
	configRoot, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolver directorio de configuración de memoria: %w", err)
	}
	return filepath.Join(configRoot, "ai-engineering-os", "memory"), nil
}

func injectRunMemory(
	ctx context.Context,
	provider memory.ProjectMemory,
	projectID string,
	task string,
	units []execution.WorkUnit,
) ([]execution.WorkUnit, memory.RecallContext) {
	if provider == nil {
		return append([]execution.WorkUnit(nil), units...), memory.RecallContext{Project: projectID}
	}
	recall := memory.RecallBestEffort(
		ctx,
		provider,
		projectID,
		task,
		memory.DefaultRecallLimit,
		memory.DefaultMemoryContextMax,
	)
	out := append([]execution.WorkUnit(nil), units...)
	if !recall.Used || strings.TrimSpace(recall.Context) == "" {
		return out, recall
	}
	for i := range out {
		out[i].CollaborationContext = coordinator.MergeContext(
			out[i].CollaborationContext,
			recall.Context,
			coordinator.DefaultCombinedContextMax,
		)
	}
	return out, recall
}

func persistReadOnlyRunMemory(
	ctx context.Context,
	provider memory.ProjectMemory,
	projectID string,
	runID string,
	task string,
	intent string,
	store *artifact.Store,
	result coordinator.Result,
) runMemoryReport {
	report := runMemoryReport{}
	if provider == nil {
		return report
	}
	if store != nil {
		values, err := store.ListRun(runID)
		if err != nil {
			report.Artifact = memory.PersistReport{
				Provider: provider.ID(),
				Project: projectID,
				Warnings: []string{"list run artifacts: " + err.Error()},
			}
		} else {
			report.Artifact = memory.PersistCuratedArtifactsBestEffort(
				ctx, provider, projectID, values,
			)
		}
	}
	if result.Completed &&
		result.Progress.Failed == 0 &&
		result.Progress.Blocked == 0 &&
		result.Progress.Cancelled == 0 {
		learning := memory.RunLearning{
			RunID:  runID,
			Task:   task,
			Intent: intent,
		}
		for _, item := range result.Tasks {
			if !item.Completed || strings.TrimSpace(item.Summary) == "" {
				continue
			}
			learning.TaskSummaries = append(learning.TaskSummaries, memory.TaskLearning{
				TaskID: item.TaskID,
				AgentID: item.AgentID,
				Summary: item.Summary,
			})
		}
		report.Learning = memory.PersistRunLearningBestEffort(
			ctx, provider, projectID, learning,
		)
	}
	return report
}

func persistMutationRunMemory(
	ctx context.Context,
	provider memory.ProjectMemory,
	projectID string,
	runID string,
	task string,
	intent string,
	store *artifact.Store,
	changeCount int,
	qualityReason string,
) runMemoryReport {
	report := runMemoryReport{}
	if provider == nil {
		return report
	}
	if store != nil {
		values, err := store.ListRun(runID)
		if err != nil {
			report.Artifact = memory.PersistReport{
				Provider: provider.ID(),
				Project: projectID,
				Warnings: []string{"list run artifacts: " + err.Error()},
			}
		} else {
			report.Artifact = memory.PersistCuratedArtifactsBestEffort(
				ctx, provider, projectID, values,
			)
		}
	}
	report.Learning = memory.PersistRunLearningBestEffort(
		ctx,
		provider,
		projectID,
		memory.RunLearning{
			RunID:  runID,
			Task:   task,
			Intent: intent,
			TaskSummaries: []memory.TaskLearning{{
				TaskID:  "integration-quality",
				AgentID: "quality-gate",
				Summary: fmt.Sprintf(
					"Integrated %d change(s); quality passed (%s); contract/architecture gate passed; promotion candidate ready for explicit approval.",
					changeCount,
					strings.TrimSpace(qualityReason),
				),
			}},
		},
	)
	return report
}

func printMemorySummary(recall memory.RecallContext, report runMemoryReport) {
	if recall.Used {
		fmt.Printf("Memory recall: %s (used)\n", recall.Provider)
	} else if strings.TrimSpace(recall.Warning) != "" {
		fmt.Printf("Memory recall: unavailable (%s)\n", recall.Warning)
	}
	if report.Artifact.Saved > 0 || report.Learning.Saved > 0 {
		fmt.Printf(
			"Memory persisted: artifacts=%d run-learning=%d\n",
			report.Artifact.Saved,
			report.Learning.Saved,
		)
	}
	for _, warning := range append(
		append([]string(nil), report.Artifact.Warnings...),
		report.Learning.Warnings...,
	) {
		fmt.Printf("Memory warning: %s\n", warning)
	}
}
