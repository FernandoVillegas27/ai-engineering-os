package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/FernandoVillegas27/ai-engineering-os/core/promotion"
	"github.com/FernandoVillegas27/ai-engineering-os/core/promotioncandidate"
	"github.com/FernandoVillegas27/ai-engineering-os/core/staging"
)

type runPromotionOptions struct {
	runID        string
	promotionDir string
	json         bool
}

type promotionView struct {
	RunID               string                    `json:"run_id"`
	OriginalRoot        string                    `json:"original_root"`
	IntegratedWorkspace string                    `json:"integrated_workspace"`
	QualityPassed       bool                      `json:"quality_passed"`
	PreImageDigest      string                    `json:"pre_image_digest"`
	Changes             []staging.WorkspaceChange `json:"changes"`
	Ready               bool                      `json:"ready"`
	CreatedAt           string                    `json:"created_at"`
}

func runRunPromotion(args []string) error {
	options, err := parseRunPromotionArgs(args)
	if err != nil {
		return err
	}
	store, err := openPromotionCandidateStore(options.promotionDir)
	if err != nil {
		return err
	}
	record, err := store.Load(options.runID)
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

	view := promotionView{
		RunID:               record.RunID,
		OriginalRoot:        record.OriginalRoot,
		IntegratedWorkspace: record.StagedWorkspace,
		QualityPassed:       record.QualityPassed,
		PreImageDigest:      record.PreImageDigest,
		Changes:             append([]staging.WorkspaceChange(nil), record.Declared...),
		Ready:               plan.Ready,
		CreatedAt:           record.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
	if options.json {
		return printJSON(view)
	}

	fmt.Println("Promotion Candidate")
	fmt.Printf("Run ID: %s\n", view.RunID)
	fmt.Printf("Quality passed: %t\n", view.QualityPassed)
	fmt.Printf("Ready: %t\n", view.Ready)
	fmt.Printf("Original: %s\n", view.OriginalRoot)
	fmt.Printf("Integrated workspace: %s\n", view.IntegratedWorkspace)
	fmt.Printf("Changes: %d\n", len(view.Changes))
	for _, change := range view.Changes {
		fmt.Printf("  - [%s] %s\n", change.Action, change.Path)
	}
	fmt.Printf("Created: %s\n", view.CreatedAt)
	fmt.Println("Approval required: YES")
	fmt.Println("No files were applied.")
	return nil
}

func parseRunPromotionArgs(args []string) (runPromotionOptions, error) {
	var options runPromotionOptions
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--promotion-dir":
			if i+1 >= len(args) {
				return runPromotionOptions{}, fmt.Errorf("--promotion-dir requiere una ruta")
			}
			i++
			options.promotionDir = args[i]
		case "--json":
			options.json = true
		case "-h", "--help":
			return runPromotionOptions{}, fmt.Errorf("uso: ai run promotion <run-id> [--promotion-dir ruta] [--json]")
		default:
			if strings.HasPrefix(args[i], "-") {
				return runPromotionOptions{}, fmt.Errorf("opción desconocida: %s", args[i])
			}
			if options.runID != "" {
				return runPromotionOptions{}, fmt.Errorf("se esperaba un solo run id")
			}
			options.runID = args[i]
		}
	}
	if strings.TrimSpace(options.runID) == "" {
		return runPromotionOptions{}, fmt.Errorf("run id es obligatorio")
	}
	return options, nil
}

func openPromotionCandidateStore(explicit string) (*promotioncandidate.Store, error) {
	root, err := resolveRunAuxDir(explicit, "AI_ENGINEERING_OS_PROMOTION_DIR", "promotion-candidates")
	if err != nil {
		return nil, err
	}
	return promotioncandidate.NewStore(root)
}
