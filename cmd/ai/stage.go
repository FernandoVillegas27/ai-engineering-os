package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/classifier"
	"github.com/FernandoVillegas27/ai-engineering-os/core/detector"
	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/orchestration"
	"github.com/FernandoVillegas27/ai-engineering-os/core/proposal"
	"github.com/FernandoVillegas27/ai-engineering-os/core/quality"
	"github.com/FernandoVillegas27/ai-engineering-os/core/registry"
	"github.com/FernandoVillegas27/ai-engineering-os/core/staging"
	codexintegration "github.com/FernandoVillegas27/ai-engineering-os/integrations/codex"
	"github.com/FernandoVillegas27/ai-engineering-os/integrations/localverify"
)

type stageOptions struct {
	root    string
	task    string
	useAI   bool
	runtime string
}

func runStage(args []string) error {
	options, err := parseStageArgs(args)
	if err != nil {
		return err
	}

	root, err := filepath.Abs(options.root)
	if err != nil {
		return fmt.Errorf("resolver ruta del proyecto: %w", err)
	}

	capabilitiesPath, err := resolveRegistryFile("capabilities.yaml")
	if err != nil {
		return err
	}
	agentsPath, err := resolveRegistryFile("agents.yaml")
	if err != nil {
		return err
	}
	skillsPath, err := resolveRegistryFile("skills.yaml")
	if err != nil {
		return err
	}
	toolsPath, err := resolveRegistryFile("tools.yaml")
	if err != nil {
		return err
	}

	capabilities, err := registry.LoadCapabilities(capabilitiesPath)
	if err != nil {
		return err
	}
	agents, err := registry.LoadAgents(agentsPath)
	if err != nil {
		return err
	}
	skills, err := registry.LoadSkills(skillsPath)
	if err != nil {
		return err
	}
	tools, err := registry.LoadTools(toolsPath)
	if err != nil {
		return err
	}
	detection, err := detector.Inspect(root)
	if err != nil {
		return fmt.Errorf("inspeccionar proyecto: %w", err)
	}

	var classified classifier.Result
	if options.useAI {
		resolution, err := classifier.Resolve(
			context.Background(),
			options.task,
			capabilities,
			readNormalizedProjectContext(root),
			codexintegration.New(),
		)
		if err != nil {
			return err
		}
		printClassificationResolution(resolution)
		fmt.Println()
		if resolution.ClarificationRequired {
			return fmt.Errorf("se requiere aclaración antes de preparar cambios: %s", resolution.ClarificationQuestion)
		}
		if resolution.Result.NeedsAI {
			return fmt.Errorf("la tarea sigue ambigua y no pudo clasificarse")
		}
		classified = resolution.Result
	} else {
		classified, err = classifier.Classify(options.task, capabilities)
		if err != nil {
			return err
		}
		printClassification(classified)
		fmt.Println()
		if classified.NeedsAI {
			return fmt.Errorf("la tarea es ambigua; repetí con --ai para habilitar el fallback de Codex")
		}
	}

	plan, err := orchestration.Build(
		orchestration.Request{
			Capabilities: classified.Capabilities,
			Intent:       classified.Intent,
		},
		detection,
		agents,
		skills,
		tools,
	)
	if err != nil {
		return err
	}

	units, err := execution.BuildWorkUnits(options.task, root, plan, agents)
	if err != nil {
		return err
	}
	if len(units) != 1 {
		return fmt.Errorf("controlled staging v1 requires exactly one work unit, got %d", len(units))
	}

	var proposalRuntime proposal.Runtime
	switch options.runtime {
	case "codex":
		r := codexintegration.NewProposalRuntime()
		proposalRuntime = r
	default:
		return fmt.Errorf("runtime no soportado: %s", options.runtime)
	}

	unit := units[0]
	generated, err := (proposal.Generator{Runtime: proposalRuntime}).Generate(context.Background(), unit)
	if err != nil {
		return err
	}
	declaredFiles, err := staging.ValidateDeclaredFiles(generated.Files, execution.DefaultSafeViewPolicy())
	if err != nil {
		return fmt.Errorf("la propuesta generada no es segura para staging: %w", err)
	}

	fmt.Println("Controlled Stage Proposal")
	fmt.Println()
	fmt.Printf("Proyecto original: %s\n", root)
	fmt.Printf("Agent: %s\n", unit.AgentID)
	fmt.Printf("Runtime: %s\n", proposalRuntime.ID())
	fmt.Printf("Summary: %s\n", generated.Summary)

	if len(generated.Files) > 0 {
		fmt.Println("Files:")
		for _, file := range generated.Files {
			fmt.Printf("  - [%s] %s: %s\n", file.Action, file.Path, file.Rationale)
		}
	}
	if len(generated.Risks) > 0 {
		fmt.Println("Risks:")
		for _, risk := range generated.Risks {
			fmt.Printf("  - [%s] %s\n", risk.Severity, risk.Detail)
		}
	}
	if len(generated.Verification) > 0 {
		fmt.Println("Verification:")
		for _, step := range generated.Verification {
			fmt.Printf("  - %s\n", step)
		}
	}
	fmt.Println()
	fmt.Printf("Propuesta validada para staging aislado: %d archivo(s).\n", len(declaredFiles))
	fmt.Println("Todavía NO se modificó ningún archivo.")
	fmt.Print("Escribí APPLY para aplicarlo únicamente en un workspace aislado: ")

	reader := bufio.NewReader(os.Stdin)
	answer, err := reader.ReadString('\n')
	if err != nil && strings.TrimSpace(answer) == "" {
		return fmt.Errorf("leer aprobación: %w", err)
	}
	if strings.TrimSpace(answer) != "APPLY" {
		fmt.Println("Operación cancelada. El proyecto original no fue modificado.")
		return nil
	}

	stageRuntime := codexintegration.NewStageRuntime()
	result, err := stageRuntime.Stage(context.Background(), unit, generated)
	if err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("Controlled Stage Completed")
	fmt.Printf("Workspace aislado: %s\n", result.Workspace)
	fmt.Printf("Archivos cambiados: %d\n", len(result.Changes))
	for _, file := range result.Changes {
		fmt.Printf("  - [%s] %s\n", file.Action, file.Path)
	}
	fmt.Println()
	fmt.Println("El proyecto original no fue modificado. Iniciando Quality Loop en el workspace aislado...")

	allowedFiles := make([]string, 0, len(declaredFiles))
	for _, file := range declaredFiles {
		allowedFiles = append(allowedFiles, file.Path)
	}

	qualityResult, err := (quality.Loop{
		Verifier:    localverify.New(),
		Reviewer:    codexintegration.NewReviewRuntime(),
		Corrector:   codexintegration.NewCorrectionRuntime(),
		MaxAttempts: 3,
	}).Run(context.Background(), result.Workspace, options.task, allowedFiles)
	if err != nil {
		return fmt.Errorf("quality loop: %w", err)
	}

	fmt.Println()
	fmt.Println("Quality Loop")
	for _, attempt := range qualityResult.Attempts {
		fmt.Printf("Intento %d\n", attempt.Number)
		fmt.Printf("  Verification: %s\n", attempt.Verification.Status)
		for _, check := range attempt.Verification.Checks {
			status := "PASS"
			if !check.Passed {
				status = "FAIL"
			}
			fmt.Printf("    - [%s] %s", status, check.Name)
			if check.Duration > 0 {
				fmt.Printf(" (%s)", check.Duration.Round(time.Millisecond))
			}
			fmt.Println()
			if !check.Passed {
				if detail := strings.TrimSpace(check.Stderr); detail != "" {
					fmt.Printf("      %s\n", strings.ReplaceAll(detail, "\n", "\n      "))
				}
			}
		}
		fmt.Printf("  Review: %s\n", attempt.Review.Verdict)
		if strings.TrimSpace(attempt.Review.Summary) != "" {
			fmt.Printf("    %s\n", attempt.Review.Summary)
		}
		for _, finding := range attempt.Review.Findings {
			fmt.Printf("    - [%s] %s: %s\n", finding.Severity, finding.Title, finding.Detail)
		}
		if attempt.Corrected {
			fmt.Println("  Correction: aplicada en el workspace aislado")
		}
	}

	fmt.Println()
	if !qualityResult.Passed {
		fmt.Println("Quality Gate FAILED")
		fmt.Println("El workspace aislado se conserva para inspección. El proyecto original sigue intacto.")
		return fmt.Errorf("quality gate failed after %d attempt(s)", len(qualityResult.Attempts))
	}

	fmt.Println("Quality Gate PASSED")
	fmt.Println("El workspace aislado quedó verificado y aprobado por revisión independiente.")
	fmt.Println("El proyecto original no fue modificado. No se hizo commit ni push.")
	return nil
}

func parseStageArgs(args []string) (stageOptions, error) {
	options := stageOptions{root: ".", runtime: "codex"}
	var positional []string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--task":
			if i+1 >= len(args) {
				return stageOptions{}, fmt.Errorf("--task requiere texto")
			}
			i++
			options.task = args[i]
		case "--ai":
			options.useAI = true
		case "--runtime":
			if i+1 >= len(args) {
				return stageOptions{}, fmt.Errorf("--runtime requiere un valor")
			}
			i++
			options.runtime = args[i]
		case "-h", "--help":
			return stageOptions{}, fmt.Errorf("uso: ai stage [ruta-del-proyecto] --task <tarea> [--runtime codex] [--ai]")
		default:
			if strings.HasPrefix(args[i], "-") {
				return stageOptions{}, fmt.Errorf("opción desconocida: %s", args[i])
			}
			positional = append(positional, args[i])
		}
	}

	if len(positional) > 1 {
		return stageOptions{}, fmt.Errorf("se esperaba una sola ruta de proyecto")
	}
	if len(positional) == 1 {
		options.root = positional[0]
	}
	if strings.TrimSpace(options.task) == "" {
		return stageOptions{}, fmt.Errorf("--task es obligatorio")
	}
	if options.runtime != "codex" {
		return stageOptions{}, fmt.Errorf("runtime no soportado: %s", options.runtime)
	}
	return options, nil
}
