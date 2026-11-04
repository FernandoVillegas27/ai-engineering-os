package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/controlplane"
	"github.com/FernandoVillegas27/ai-engineering-os/core/promotion"
	"github.com/FernandoVillegas27/ai-engineering-os/core/runstate"
)

type runPromoteOptions struct {
	runID        string
	promotionDir string
	stateDir     string
	eventsDir    string
	approve      bool
	json         bool
}

func runRunPromote(args []string) error {
	options, err := parseRunPromoteArgs(args)
	if err != nil {
		return err
	}
	if !options.approve {
		return fmt.Errorf("promoción bloqueada: agregá --approve para aprobar explícitamente la aplicación")
	}

	candidateStore, err := openPromotionCandidateStore(options.promotionDir)
	if err != nil {
		return err
	}
	record, err := candidateStore.Load(options.runID)
	if err != nil {
		return fmt.Errorf("load promotion candidate: %w", err)
	}
	request, err := record.Request()
	if err != nil {
		return fmt.Errorf("reconstruct promotion candidate: %w", err)
	}

	plan, err := promotion.DryRun(context.Background(), request)
	if err != nil {
		return fmt.Errorf("promotion candidate is no longer ready: %w", err)
	}
	if !plan.Ready {
		return fmt.Errorf("promotion candidate is not ready")
	}

	result, err := promotion.Apply(context.Background(), promotion.ApplyRequest{
		Request:  request,
		Approved: true,
	})
	if err != nil {
		return fmt.Errorf("apply promotion: %w", err)
	}
	if !result.Applied {
		return fmt.Errorf("promotion did not apply")
	}

	now := time.Now().UTC()
	if err := recordPromotionApproval(options, now, len(result.Changes)); err != nil {
		return err
	}

	if options.json {
		return printJSON(struct {
			RunID       string `json:"run_id"`
			Applied     bool   `json:"applied"`
			ChangeCount int    `json:"change_count"`
			RolledBack  bool   `json:"rolled_back"`
		}{
			RunID:       options.runID,
			Applied:     result.Applied,
			ChangeCount: len(result.Changes),
			RolledBack:  result.RolledBack,
		})
	}

	fmt.Println("Promotion Applied")
	fmt.Printf("Run ID: %s\n", options.runID)
	fmt.Printf("Changes applied: %d\n", len(result.Changes))
	fmt.Printf("Rollback triggered: %t\n", result.RolledBack)
	fmt.Println("Original project now matches the approved integrated workspace.")
	fmt.Println("No Git commit or push was performed.")
	return nil
}

func recordPromotionApproval(options runPromoteOptions, now time.Time, changeCount int) error {
	runStore, err := openRunStore(options.stateDir)
	if err != nil {
		return err
	}
	run, err := runStore.Load(options.runID)
	if err != nil {
		return fmt.Errorf("load durable run for approval audit: %w", err)
	}
	run.Approvals = append(run.Approvals, runstate.Approval{
		Kind:      "promotion",
		Status:    "granted",
		CreatedAt: now,
		UpdatedAt: now,
	})
	run.UpdatedAt = now
	if err := runStore.Save(run); err != nil {
		return fmt.Errorf("persist promotion approval audit: %w", err)
	}

	eventStore, err := openControlPlaneStore(options.eventsDir)
	if err != nil {
		return err
	}
	event, err := controlplane.NewEvent(controlplane.NewEventRequest{
		RunID:   options.runID,
		Type:    controlplane.EventApprovalGranted,
		Summary: fmt.Sprintf("promotion applied: %d change(s)", changeCount),
		Data: map[string]any{
			"kind":         "promotion",
			"change_count": changeCount,
		},
	}, now)
	if err != nil {
		return err
	}
	if _, err := eventStore.Append(context.Background(), event); err != nil {
		return fmt.Errorf("persist promotion approval event: %w", err)
	}
	return nil
}

func parseRunPromoteArgs(args []string) (runPromoteOptions, error) {
	var options runPromoteOptions
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--approve":
			options.approve = true
		case "--promotion-dir":
			if i+1 >= len(args) {
				return runPromoteOptions{}, fmt.Errorf("--promotion-dir requiere una ruta")
			}
			i++
			options.promotionDir = args[i]
		case "--state-dir":
			if i+1 >= len(args) {
				return runPromoteOptions{}, fmt.Errorf("--state-dir requiere una ruta")
			}
			i++
			options.stateDir = args[i]
		case "--events-dir":
			if i+1 >= len(args) {
				return runPromoteOptions{}, fmt.Errorf("--events-dir requiere una ruta")
			}
			i++
			options.eventsDir = args[i]
		case "--json":
			options.json = true
		case "-h", "--help":
			return runPromoteOptions{}, fmt.Errorf("uso: ai run promote <run-id> --approve [--promotion-dir ruta] [--state-dir ruta] [--events-dir ruta] [--json]")
		default:
			if strings.HasPrefix(args[i], "-") {
				return runPromoteOptions{}, fmt.Errorf("opción desconocida: %s", args[i])
			}
			if options.runID != "" {
				return runPromoteOptions{}, fmt.Errorf("se esperaba un solo run id")
			}
			options.runID = args[i]
		}
	}
	if strings.TrimSpace(options.runID) == "" {
		return runPromoteOptions{}, fmt.Errorf("run id es obligatorio")
	}
	return options, nil
}
