package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/FernandoVillegas27/ai-engineering-os/core/classifier"
	"github.com/FernandoVillegas27/ai-engineering-os/core/detector"
	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/orchestration"
	"github.com/FernandoVillegas27/ai-engineering-os/core/registry"
	codexintegration "github.com/FernandoVillegas27/ai-engineering-os/integrations/codex"
)

type executeOptions struct {
	root    string
	task    string
	useAI   bool
	dryRun  bool
	runtime string
}

func runExecute(args []string) error {
	options, err := parseExecuteArgs(args)
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
			return fmt.Errorf("se requiere aclaración antes de ejecutar: %s", resolution.ClarificationQuestion)
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

	var executor execution.Executor
	title := "Execution Dry Run"
	if options.dryRun {
		executor = execution.DryRunExecutor{}
	} else {
		switch options.runtime {
		case "codex":
			executor = execution.RuntimeExecutor{Runtime: codexintegration.NewExecutionRuntime()}
			title = "Execution Read-Only Runtime"
		default:
			return fmt.Errorf("runtime no soportado: %s", options.runtime)
		}
	}

	fmt.Println(title)
	fmt.Println()
	fmt.Printf("Proyecto: %s\n", root)
	fmt.Printf("Intent: %s\n", plan.Intent)
	fmt.Printf("Work units: %d\n", len(units))

	for _, unit := range units {
		result, err := executor.Execute(context.Background(), unit)
		if err != nil {
			return err
		}

		fmt.Printf("  - %s\n", unit.ID)
		fmt.Printf("    agent: %s\n", unit.AgentID)
		fmt.Printf("    capabilities: %s\n", strings.Join(unit.Capabilities, ", "))
		if len(unit.Skills) > 0 {
			fmt.Printf("    skills enabled: %s\n", strings.Join(unit.Skills, ", "))
		}
		if len(unit.Tools) > 0 {
			fmt.Printf("    tools logical: %s\n", strings.Join(unit.Tools, ", "))
		}
		fmt.Printf("    mode: %s\n", unit.Mode)
		fmt.Printf("    source_code: %s\n", unit.Permissions.SourceCode)
		fmt.Printf("    tests: %s\n", unit.Permissions.Tests)
		fmt.Printf("    git_commit: %s\n", unit.Permissions.GitCommit)
		fmt.Printf("    git_push: %s\n", unit.Permissions.GitPush)
		fmt.Printf("    status: %s\n", result.Status)
		fmt.Printf("    executed: %t\n", result.Executed)
		if result.Runtime != "" {
			fmt.Printf("    runtime: %s\n", result.Runtime)
		}
		fmt.Printf("    changes: %d\n", len(result.Changes))
		if strings.TrimSpace(result.Summary) != "" {
			fmt.Printf("    summary: %s\n", result.Summary)
		}
		if len(result.Findings) > 0 {
			fmt.Println("    findings:")
			for _, finding := range result.Findings {
				fmt.Printf("      - [%s] %s: %s\n", finding.Severity, finding.Title, finding.Detail)
			}
		}
		if len(result.Evidence) > 0 {
			fmt.Println("    evidence:")
			for _, evidence := range result.Evidence {
				fmt.Printf("      - %s: %s\n", evidence.Path, evidence.Detail)
			}
		}
	}

	fmt.Println()
	if options.dryRun {
		fmt.Println("Dry-run completado: ningún runtime fue invocado y el proyecto no fue modificado.")
	} else {
		fmt.Println("Ejecución read-only completada: el runtime analizó el proyecto sin modificarlo.")
	}
	return nil
}

func parseExecuteArgs(args []string) (executeOptions, error) {
	options := executeOptions{root: "."}
	var positional []string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--task":
			if i+1 >= len(args) {
				return executeOptions{}, fmt.Errorf("--task requiere texto")
			}
			i++
			options.task = args[i]
		case "--ai":
			options.useAI = true
		case "--dry-run":
			options.dryRun = true
		case "--runtime":
			if i+1 >= len(args) {
				return executeOptions{}, fmt.Errorf("--runtime requiere un valor")
			}
			i++
			options.runtime = args[i]
		case "-h", "--help":
			return executeOptions{}, fmt.Errorf("uso: ai execute [ruta-del-proyecto] --task <tarea> (--dry-run | --runtime codex) [--ai]")
		default:
			if strings.HasPrefix(args[i], "-") {
				return executeOptions{}, fmt.Errorf("opción desconocida: %s", args[i])
			}
			positional = append(positional, args[i])
		}
	}

	if len(positional) > 1 {
		return executeOptions{}, fmt.Errorf("se esperaba una sola ruta de proyecto")
	}
	if len(positional) == 1 {
		options.root = positional[0]
	}
	if strings.TrimSpace(options.task) == "" {
		return executeOptions{}, fmt.Errorf("--task es obligatorio")
	}
	if options.dryRun && options.runtime != "" {
		return executeOptions{}, fmt.Errorf("--dry-run y --runtime no se pueden usar juntos")
	}
	if !options.dryRun && options.runtime == "" {
		return executeOptions{}, fmt.Errorf("indicá --dry-run o --runtime codex")
	}
	if options.runtime != "" && options.runtime != "codex" {
		return executeOptions{}, fmt.Errorf("runtime no soportado: %s", options.runtime)
	}

	return options, nil
}
