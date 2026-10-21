package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/FernandoVillegas27/ai-engineering-os/core/classifier"
	"github.com/FernandoVillegas27/ai-engineering-os/core/detector"
	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/modelpolicy"
	"github.com/FernandoVillegas27/ai-engineering-os/core/orchestration"
	"github.com/FernandoVillegas27/ai-engineering-os/core/registry"
	codexintegration "github.com/FernandoVillegas27/ai-engineering-os/integrations/codex"
)

type proposeOptions struct {
	root        string
	task        string
	useAI       bool
	runtime     string
	modelPolicy modelpolicy.Policy
	model       string
}

func runPropose(args []string) error {
	options, err := parseProposeArgs(args)
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
			return fmt.Errorf("se requiere aclaración antes de proponer cambios: %s", resolution.ClarificationQuestion)
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

	routing := modelRoutingOptions{Policy: options.modelPolicy, Override: options.model}

	fmt.Println("Proposed Changes")
	fmt.Println()
	fmt.Printf("Proyecto: %s\n", root)
	fmt.Printf("Intent: %s\n", plan.Intent)
	fmt.Printf("Work units: %d\n", len(units))

	for _, unit := range units {
		generated, err := generateProposalWithRouting(context.Background(), unit, options.runtime, routing)
		if err != nil {
			return err
		}
		result := generated.Proposal

		fmt.Printf("  - %s\n", unit.ID)
		fmt.Printf("    agent: %s\n", unit.AgentID)
		fmt.Printf("    capabilities: %s\n", strings.Join(unit.Capabilities, ", "))
		fmt.Printf("    runtime: %s\n", generated.RuntimeID)
		fmt.Printf("    model policy: %s\n", options.modelPolicy)
		fmt.Printf("    model: %s\n", displayModel(generated.Model))
		if generated.Escalated {
			fmt.Println("    model escalation: sí (proposal drift detectado)")
		}
		fmt.Printf("    summary: %s\n", result.Summary)

		if len(result.Files) > 0 {
			fmt.Println("    files:")
			for _, file := range result.Files {
				fmt.Printf("      - [%s] %s: %s\n", file.Action, file.Path, file.Rationale)
			}
		}
		if len(result.Risks) > 0 {
			fmt.Println("    risks:")
			for _, risk := range result.Risks {
				fmt.Printf("      - [%s] %s\n", risk.Severity, risk.Detail)
			}
		}
		if len(result.Verification) > 0 {
			fmt.Println("    verification:")
			for _, step := range result.Verification {
				fmt.Printf("      - %s\n", step)
			}
		}
		if strings.TrimSpace(result.Patch) != "" {
			fmt.Println("    patch:")
			for _, line := range strings.Split(result.Patch, "\n") {
				fmt.Printf("      %s\n", line)
			}
		} else {
			fmt.Println("    patch: vacío")
		}
	}

	fmt.Println()
	fmt.Println("Propuesta completada: el runtime generó un diff como texto y no modificó el proyecto.")
	return nil
}

func parseProposeArgs(args []string) (proposeOptions, error) {
	options := proposeOptions{root: ".", runtime: "auto", modelPolicy: modelpolicy.PolicyAuto}
	var positional []string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--task":
			if i+1 >= len(args) {
				return proposeOptions{}, fmt.Errorf("--task requiere texto")
			}
			i++
			options.task = args[i]
		case "--ai":
			options.useAI = true
		case "--runtime":
			if i+1 >= len(args) {
				return proposeOptions{}, fmt.Errorf("--runtime requiere un valor")
			}
			i++
			options.runtime = args[i]
		case "--model-policy":
			if i+1 >= len(args) {
				return proposeOptions{}, fmt.Errorf("--model-policy requiere un valor")
			}
			i++
			policy, err := modelpolicy.Parse(args[i])
			if err != nil {
				return proposeOptions{}, err
			}
			options.modelPolicy = policy
		case "--model":
			if i+1 >= len(args) {
				return proposeOptions{}, fmt.Errorf("--model requiere un id")
			}
			i++
			options.model = strings.TrimSpace(args[i])
		case "-h", "--help":
			return proposeOptions{}, fmt.Errorf("uso: ai propose [ruta-del-proyecto] --task <tarea> [--runtime auto|codex|cursor] [--model-policy auto|economy|balanced|deep] [--model id] [--ai]")
		default:
			if strings.HasPrefix(args[i], "-") {
				return proposeOptions{}, fmt.Errorf("opción desconocida: %s", args[i])
			}
			positional = append(positional, args[i])
		}
	}

	if len(positional) > 1 {
		return proposeOptions{}, fmt.Errorf("se esperaba una sola ruta de proyecto")
	}
	if len(positional) == 1 {
		options.root = positional[0]
	}
	if strings.TrimSpace(options.task) == "" {
		return proposeOptions{}, fmt.Errorf("--task es obligatorio")
	}
	if options.runtime != "auto" && options.runtime != "codex" && options.runtime != "cursor" {
		return proposeOptions{}, fmt.Errorf("runtime no soportado: %s", options.runtime)
	}
	return options, nil
}
