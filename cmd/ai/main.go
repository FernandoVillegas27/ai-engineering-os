package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/FernandoVillegas27/ai-engineering-os/core/classifier"
	"github.com/FernandoVillegas27/ai-engineering-os/core/detector"
	"github.com/FernandoVillegas27/ai-engineering-os/core/doctor"
	"github.com/FernandoVillegas27/ai-engineering-os/core/orchestration"
	"github.com/FernandoVillegas27/ai-engineering-os/core/project"
	"github.com/FernandoVillegas27/ai-engineering-os/core/registry"
	context7integration "github.com/FernandoVillegas27/ai-engineering-os/integrations/context7"
	engramintegration "github.com/FernandoVillegas27/ai-engineering-os/integrations/engram"
	codexintegration "github.com/FernandoVillegas27/ai-engineering-os/integrations/codex"
	gentleaiintegration "github.com/FernandoVillegas27/ai-engineering-os/integrations/gentleai"
)

type inspectOptions struct {
	root         string
	registryPath string
	write        bool
}

type doctorOptions struct {
	root         string
	registryPath string
}

type planOptions struct {
	root         string
	capabilities []string
	intent       orchestration.Intent
	task         string
	useAI        bool
}

type classifyOptions struct {
	task  string
	useAI bool
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "inspect":
		if err := runInspect(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "run":
		if err := runRun(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "memory":
		if err := runMemory(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "docs":
		if err := runDocs(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "doctor":
		if err := runDoctor(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "plan":
		if err := runPlan(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "classify":
		if err := runClassify(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "execute":
		if err := runExecute(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "propose":
		if err := runPropose(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "stage":
		if err := runStage(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "gentle":
		if err := runGentle(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "diagnose":
		if err := runDiagnose(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Comando desconocido: %s\n\n", os.Args[1])
		printUsage()
		os.Exit(2)
	}
}

func runInspect(args []string) error {
	options, err := parseInspectArgs(args)
	if err != nil {
		return err
	}

	resolvedRegistry, err := resolveSkillRegistry(options.registryPath)
	if err != nil {
		return err
	}

	context, err := project.Build(options.root, resolvedRegistry)
	if err != nil {
		return err
	}

	if options.write {
		outputPath, err := project.Write(options.root, context)
		if err != nil {
			return err
		}
		absolutePath, _ := filepath.Abs(outputPath)
		fmt.Printf("Proyecto inspeccionado correctamente.\nContexto generado en: %s\n", absolutePath)
		return nil
	}

	data, err := project.Marshal(context)
	if err != nil {
		return err
	}
	fmt.Print(string(data))
	return nil
}



func runClassify(args []string) error {
	options, err := parseClassifyArgs(args)
	if err != nil {
		return err
	}

	capabilitiesPath, err := resolveRegistryFile("capabilities.yaml")
	if err != nil {
		return err
	}
	capabilities, err := registry.LoadCapabilities(capabilitiesPath)
	if err != nil {
		return err
	}

	if !options.useAI {
		result, err := classifier.Classify(options.task, capabilities)
		if err != nil {
			return err
		}
		printClassification(result)
		return nil
	}

	resolution, err := classifier.Resolve(
		context.Background(),
		options.task,
		capabilities,
		"",
		codexintegration.New(),
	)
	if err != nil {
		return err
	}

	printClassificationResolution(resolution)
	return nil
}


func printClassificationResolution(resolution classifier.Resolution) {
	printClassification(resolution.Result)
	fmt.Printf("Fuente: %s\n", resolution.Source)
	if resolution.Provider != "" {
		fmt.Printf("Provider: %s\n", resolution.Provider)
	}
	if resolution.ClarificationRequired {
		fmt.Printf("Aclaración requerida: %s\n", resolution.ClarificationQuestion)
	}
}

func printClassification(result classifier.Result) {
	fmt.Println("Task Classification")
	fmt.Println()
	fmt.Printf("Tarea: %s\n", result.Task)
	if result.Intent != "" {
		fmt.Printf("Intent: %s\n", result.Intent)
	} else {
		fmt.Println("Intent: sin resolver")
	}
	if len(result.Capabilities) > 0 {
		fmt.Printf("Capabilities: %s\n", strings.Join(result.Capabilities, ", "))
	} else {
		fmt.Println("Capabilities: sin resolver")
	}
	fmt.Printf("Confianza: %s\n", result.Confidence)
	if result.NeedsAI {
		fmt.Println("Escalado IA: requerido")
	} else {
		fmt.Println("Escalado IA: no requerido")
	}
	if len(result.Reasons) > 0 {
		fmt.Println("Evidencia:")
		for _, reason := range result.Reasons {
			fmt.Printf("  - %s\n", reason)
		}
	}
}

func runPlan(args []string) error {
	options, err := parsePlanArgs(args)
	if err != nil {
		return err
	}

	absoluteRoot, err := filepath.Abs(options.root)
	if err != nil {
		return fmt.Errorf("resolver ruta del proyecto: %w", err)
	}

	detection, err := detector.Inspect(absoluteRoot)
	if err != nil {
		return fmt.Errorf("inspeccionar proyecto: %w", err)
	}

	if options.task != "" {
		capabilitiesPath, err := resolveRegistryFile("capabilities.yaml")
		if err != nil {
			return err
		}
		capabilityRegistry, err := registry.LoadCapabilities(capabilitiesPath)
		if err != nil {
			return err
		}

		if options.useAI {
			projectContext := readNormalizedProjectContext(absoluteRoot)
			resolution, err := classifier.Resolve(
				context.Background(),
				options.task,
				capabilityRegistry,
				projectContext,
				codexintegration.New(),
			)
			if err != nil {
				return err
			}
			printClassificationResolution(resolution)
			fmt.Println()

			if resolution.ClarificationRequired {
				return fmt.Errorf("se requiere aclaración antes de planificar: %s", resolution.ClarificationQuestion)
			}
			if resolution.Result.NeedsAI {
				return fmt.Errorf("la tarea sigue ambigua y no pudo clasificarse")
			}
			options.intent = resolution.Result.Intent
			options.capabilities = resolution.Result.Capabilities
		} else {
			classification, err := classifier.Classify(options.task, capabilityRegistry)
			if err != nil {
				return err
			}
			printClassification(classification)
			fmt.Println()

			if classification.NeedsAI {
				return fmt.Errorf("la tarea es ambigua para el clasificador determinista; repetí con --ai para habilitar el fallback de Codex")
			}

			options.intent = classification.Intent
			options.capabilities = classification.Capabilities
		}
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

	agents, err := registry.LoadAgents(agentsPath)
	if err != nil {
		return err
	}
	skills, err := registry.LoadSkills(skillsPath)
	if err != nil {
		return err
	}
	toolRegistry, err := registry.LoadTools(toolsPath)
	if err != nil {
		return err
	}

	plan, err := orchestration.Build(
		orchestration.Request{
			Capabilities: options.capabilities,
			Intent:       options.intent,
		},
		detection,
		agents,
		skills,
		toolRegistry,
	)
	if err != nil {
		return err
	}

	fmt.Println("Execution Plan")
	fmt.Println()
	fmt.Printf("Proyecto: %s\n", absoluteRoot)
	fmt.Printf("Intent: %s\n", plan.Intent)
	fmt.Printf("Capabilities: %s\n", strings.Join(plan.RequestedCapabilities, ", "))

	if len(plan.Agents) == 0 {
		fmt.Println("Agentes: ninguno")
	} else {
		fmt.Println("Agentes:")
		for _, agent := range plan.Agents {
			fmt.Printf("  - %s\n", agent.AgentID)
			fmt.Printf("    capabilities: %s\n", strings.Join(agent.Capabilities, ", "))
			if len(agent.Skills.Enabled) > 0 {
				fmt.Printf("    skills enabled: %s\n", strings.Join(agent.Skills.Enabled, ", "))
			}
			if len(agent.Skills.Candidates) > 0 {
				fmt.Printf("    skills candidate: %s\n", strings.Join(agent.Skills.Candidates, ", "))
			}
			if len(agent.Tools.Required) > 0 {
				fmt.Printf("    tools required: %s\n", strings.Join(agent.Tools.Required, ", "))
			}
			if len(agent.Tools.Recommended) > 0 {
				fmt.Printf("    tools recommended: %s\n", strings.Join(agent.Tools.Recommended, ", "))
			}
		}
	}

	if len(plan.UncoveredCapabilities) > 0 {
		fmt.Printf("Sin cobertura: %s\n", strings.Join(plan.UncoveredCapabilities, ", "))
	} else {
		fmt.Println("Sin cobertura: ninguna")
	}

	fmt.Println()
	fmt.Println("Este plan no ejecuta agentes ni modifica el proyecto.")
	return nil
}

func runMemory(args []string) error {
	if len(args) == 0 || args[0] == "status" {
		client := engramintegration.New()
		status, err := client.Status(context.Background())
		if err != nil {
			return err
		}

		fmt.Println("Memoria persistente")
		fmt.Printf("Proveedor: %s\n", status.Provider)

		if !status.Available {
			fmt.Println("Estado: no instalado")
			fmt.Println("Engram no fue encontrado en PATH.")
			return nil
		}

		fmt.Println("Estado: instalado")
		if status.Version != "" {
			fmt.Printf("Versión: %s\n", status.Version)
		}
		if status.Healthy {
			fmt.Println("Diagnóstico: OK")
			return nil
		}

		fmt.Println("Diagnóstico: requiere atención")
		if status.Details != "" {
			fmt.Printf("Detalle: %s\n", status.Details)
		}
		return nil
	}

	return fmt.Errorf("subcomando de memoria desconocido: %s", args[0])
}

func runDocs(args []string) error {
	if len(args) == 0 || args[0] == "status" {
		client := context7integration.New()
		status, err := client.Status(context.Background())
		if err != nil {
			return err
		}

		fmt.Println("Documentación externa")
		fmt.Printf("Proveedor: %s\n", status.Provider)

		if !status.Available {
			fmt.Println("Estado: no disponible")
			fmt.Println("npx no fue encontrado en PATH.")
			return nil
		}

		if status.Ready {
			fmt.Println("Estado: listo")
		} else {
			fmt.Println("Estado: requiere atención")
		}
		if status.Version != "" {
			fmt.Printf("npx: %s\n", status.Version)
		}
		fmt.Printf("Paquete MCP: %s\n", context7integration.DefaultPackage)
		if !status.Ready && status.Details != "" {
			fmt.Printf("Detalle: %s\n", status.Details)
		}
		return nil
	}

	return fmt.Errorf("subcomando de documentación desconocido: %s", args[0])
}


func runGentle(args []string) error {
	client := gentleaiintegration.New()

	if len(args) == 0 || args[0] == "status" {
		status, err := client.Status(context.Background())
		if err != nil {
			return err
		}

		fmt.Println("Runtime projection")
		fmt.Printf("Proveedor: %s\n", status.Provider)

		if !status.Available {
			fmt.Println("Estado: no instalado")
			fmt.Printf("Versión estable objetivo: %s\n", gentleaiintegration.StableVersion)
			fmt.Println("Gentle-AI no fue encontrado en PATH.")
			return nil
		}

		fmt.Println("Estado: instalado")
		if status.Version != "" {
			fmt.Printf("Versión: %s\n", status.Version)
		}
		if status.Healthy {
			fmt.Println("Diagnóstico: OK")
		} else {
			fmt.Println("Diagnóstico: requiere atención")
		}
		if !status.Healthy && status.Details != "" {
			fmt.Printf("Detalle: %s\n", status.Details)
		}
		return nil
	}

	if args[0] == "plan" {
		if len(args) != 2 {
			return fmt.Errorf("uso: ai gentle plan <runtime>")
		}
		target := args[1]

		installPlan, err := client.PreviewInstall(target)
		if err != nil {
			return err
		}
		syncPlan, err := client.PreviewSync(target)
		if err != nil {
			return err
		}

		fmt.Printf("Plan de proyección para %s\n", target)
		fmt.Println()
		fmt.Println("Instalación (preview, no modifica):")
		fmt.Printf("  %s %s\n", installPlan.Command, strings.Join(installPlan.Args, " "))
		fmt.Println()
		fmt.Println("Sincronización (preview, no modifica):")
		fmt.Printf("  %s %s\n", syncPlan.Command, strings.Join(syncPlan.Args, " "))
		fmt.Println()
		fmt.Println("Aplicación real: requiere aprobación explícita.")
		return nil
	}

	if args[0] == "validate" {
		if len(args) != 2 || args[1] != "cursor" {
			return fmt.Errorf("uso: ai gentle validate cursor")
		}

		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("resolver home del usuario: %w", err)
		}

		report := gentleaiintegration.ValidateCursor(home)
		printGentleValidation(report)
		return nil
	}

	return fmt.Errorf("subcomando Gentle-AI desconocido: %s", args[0])
}


func printGentleValidation(report gentleaiintegration.ValidationReport) {
	fmt.Println("Cursor Projection")
	fmt.Println()

	warnings := 0
	errors := 0

	for _, check := range report.Checks {
		var marker string
		switch check.Level {
		case gentleaiintegration.ValidationOK:
			marker = "[OK]"
		case gentleaiintegration.ValidationWarn:
			marker = "[WARN]"
			warnings++
		case gentleaiintegration.ValidationError:
			marker = "[ERROR]"
			errors++
		}

		fmt.Printf("%-7s %-16s %s\n", marker, check.Label, check.Message)
		if check.Hint != "" && check.Level != gentleaiintegration.ValidationOK {
			fmt.Printf("        %-16s → %s\n", "", check.Hint)
		}
	}

	fmt.Println()
	switch {
	case errors > 0:
		fmt.Printf("Resultado: requiere atención (%d error/es, %d advertencia/s).\n", errors, warnings)
	case warnings > 0:
		fmt.Printf("Resultado: listo con %d advertencia/s.\n", warnings)
	default:
		fmt.Println("Resultado: listo.")
	}
}

func runDoctor(args []string) error {
	options, err := parseDoctorArgs(args)
	if err != nil {
		return err
	}

	root, err := filepath.Abs(options.root)
	if err != nil {
		return fmt.Errorf("resolver ruta del proyecto: %w", err)
	}

	checks := []doctor.Check{
		doctor.CheckWorkspace(root),
		doctor.CheckGit(root),
	}

	registryPath, registryErr := resolveSkillRegistry(options.registryPath)
	if registryErr != nil {
		checks = append(checks, doctor.Check{
			ID:      "skill-registry",
			Label:   "Skill Registry",
			Level:   doctor.LevelError,
			Message: "No se pudo resolver el Skill Registry.",
			Hint:    registryErr.Error(),
		})
	} else {
		checks = append(checks, doctor.CheckSkillRegistry(registryPath))
	}

	checks = append(checks,
		doctor.CheckProjectContext(root),
		doctor.CheckEngramProject(root),
	)

	engramClient := engramintegration.New()
	engramStatus, engramErr := engramClient.Status(context.Background())
	switch {
	case engramErr != nil:
		checks = append(checks, doctor.Check{
			ID:      "engram",
			Label:   "Engram",
			Level:   doctor.LevelWarn,
			Message: "No se pudo diagnosticar Engram.",
			Hint:    engramErr.Error(),
		})
	case !engramStatus.Available:
		checks = append(checks, doctor.Check{
			ID:      "engram",
			Label:   "Engram",
			Level:   doctor.LevelWarn,
			Message: "Engram no está instalado o no está en PATH.",
			Hint:    "Instalá Engram para habilitar memoria persistente.",
		})
	case !engramStatus.Healthy:
		checks = append(checks, doctor.Check{
			ID:      "engram",
			Label:   "Engram",
			Level:   doctor.LevelWarn,
			Message: "Engram está instalado pero requiere atención.",
			Hint:    "Ejecutá ai memory status o engram doctor --json.",
		})
	default:
		message := "Engram saludable."
		if engramStatus.Version != "" {
			message = engramStatus.Version
		}
		checks = append(checks, doctor.Check{
			ID:      "engram",
			Label:   "Engram",
			Level:   doctor.LevelOK,
			Message: message,
		})
	}

	gentleClient := gentleaiintegration.New()
	gentleStatus, gentleErr := gentleClient.Status(context.Background())
	switch {
	case gentleErr != nil:
		checks = append(checks, doctor.Check{
			ID:      "gentle-ai",
			Label:   "Gentle-AI",
			Level:   doctor.LevelWarn,
			Message: "No se pudo diagnosticar Gentle-AI.",
			Hint:    gentleErr.Error(),
		})
	case !gentleStatus.Available:
		checks = append(checks, doctor.Check{
			ID:      "gentle-ai",
			Label:   "Gentle-AI",
			Level:   doctor.LevelWarn,
			Message: "Gentle-AI todavía no está instalado.",
			Hint:    "Usá ai gentle status y ai gentle plan cursor antes de instalarlo.",
		})
	case !gentleStatus.Healthy:
		checks = append(checks, doctor.Check{
			ID:      "gentle-ai",
			Label:   "Gentle-AI",
			Level:   doctor.LevelWarn,
			Message: "Gentle-AI está instalado pero requiere atención.",
			Hint:    "Ejecutá gentle-ai doctor.",
		})
	default:
		message := "Gentle-AI saludable."
		if gentleStatus.Version != "" {
			message = gentleStatus.Version
		}
		checks = append(checks, doctor.Check{
			ID:      "gentle-ai",
			Label:   "Gentle-AI",
			Level:   doctor.LevelOK,
			Message: message,
		})
	}

	context7Client := context7integration.New()
	context7Status, context7Err := context7Client.Status(context.Background())
	switch {
	case context7Err != nil:
		checks = append(checks, doctor.Check{
			ID:      "context7",
			Label:   "Context7",
			Level:   doctor.LevelWarn,
			Message: "No se pudo diagnosticar Context7.",
			Hint:    context7Err.Error(),
		})
	case !context7Status.Available:
		checks = append(checks, doctor.Check{
			ID:      "context7",
			Label:   "Context7",
			Level:   doctor.LevelWarn,
			Message: "npx no está disponible para iniciar Context7 MCP.",
			Hint:    "Instalá Node.js/npm o corregí PATH.",
		})
	case !context7Status.Ready:
		checks = append(checks, doctor.Check{
			ID:      "context7",
			Label:   "Context7",
			Level:   doctor.LevelWarn,
			Message: "Context7 requiere atención.",
			Hint:    context7Status.Details,
		})
	default:
		checks = append(checks, doctor.Check{
			ID:      "context7",
			Label:   "Context7",
			Level:   doctor.LevelOK,
			Message: fmt.Sprintf("npx %s · %s", context7Status.Version, context7integration.DefaultPackage),
		})
	}

	printDoctorReport(doctor.Report{Checks: checks})
	return nil
}

func printDoctorReport(report doctor.Report) {
	fmt.Println("AI Engineering OS Doctor")
	fmt.Println()

	warnings := 0
	errors := 0

	for _, check := range report.Checks {
		var marker string
		switch check.Level {
		case doctor.LevelOK:
			marker = "[OK]"
		case doctor.LevelWarn:
			marker = "[WARN]"
			warnings++
		case doctor.LevelError:
			marker = "[ERROR]"
			errors++
		}

		fmt.Printf("%-7s %-16s %s\n", marker, check.Label, check.Message)
		if check.Hint != "" && check.Level != doctor.LevelOK {
			fmt.Printf("        %-16s → %s\n", "", check.Hint)
		}
	}

	fmt.Println()
	switch {
	case errors > 0:
		fmt.Printf("Resultado: requiere atención (%d error/es, %d advertencia/s).\n", errors, warnings)
	case warnings > 0:
		fmt.Printf("Resultado: listo con %d advertencia/s.\n", warnings)
	default:
		fmt.Println("Resultado: listo.")
	}
}



func parseClassifyArgs(args []string) (classifyOptions, error) {
	var options classifyOptions
	var taskParts []string

	for _, arg := range args {
		switch arg {
		case "--ai":
			options.useAI = true
		case "-h", "--help":
			return classifyOptions{}, fmt.Errorf("uso: ai classify [--ai] <tarea>")
		default:
			taskParts = append(taskParts, arg)
		}
	}

	options.task = strings.TrimSpace(strings.Join(taskParts, " "))
	if options.task == "" {
		return classifyOptions{}, fmt.Errorf("uso: ai classify [--ai] <tarea>")
	}
	return options, nil
}

func readNormalizedProjectContext(root string) string {
	path := filepath.Join(root, ".ai", "project.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	const maxContextBytes = 32 * 1024
	if len(data) > maxContextBytes {
		data = data[:maxContextBytes]
	}
	return string(data)
}

func parsePlanArgs(args []string) (planOptions, error) {
	options := planOptions{root: ".", intent: orchestration.IntentImplement}
	var positional []string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--capability":
			if i+1 >= len(args) {
				return planOptions{}, fmt.Errorf("--capability requiere un id")
			}
			i++
			options.capabilities = append(options.capabilities, args[i])
		case "--intent":
			if i+1 >= len(args) {
				return planOptions{}, fmt.Errorf("--intent requiere un valor")
			}
			i++
			options.intent = orchestration.Intent(args[i])
		case "--task":
			if i+1 >= len(args) {
				return planOptions{}, fmt.Errorf("--task requiere texto")
			}
			i++
			options.task = args[i]
		case "--ai":
			options.useAI = true
		case "-h", "--help":
			printPlanUsage()
			os.Exit(0)
		default:
			if len(args[i]) > 0 && args[i][0] == '-' {
				return planOptions{}, fmt.Errorf("opción desconocida: %s", args[i])
			}
			positional = append(positional, args[i])
		}
	}

	if len(positional) > 1 {
		return planOptions{}, fmt.Errorf("se esperaba una sola ruta de proyecto")
	}
	if len(positional) == 1 {
		options.root = positional[0]
	}
	if options.task != "" && len(options.capabilities) > 0 {
		return planOptions{}, fmt.Errorf("--task y --capability no se pueden usar juntos")
	}
	if options.task != "" && options.intent != orchestration.IntentImplement {
		return planOptions{}, fmt.Errorf("--task y --intent no se pueden usar juntos; el intent se clasifica desde la tarea")
	}
	if options.useAI && options.task == "" {
		return planOptions{}, fmt.Errorf("--ai solo puede usarse junto con --task")
	}
	if options.task == "" && len(options.capabilities) == 0 {
		return planOptions{}, fmt.Errorf("indicá --task o al menos una --capability")
	}

	return options, nil
}

func parseInspectArgs(args []string) (inspectOptions, error) {
	options := inspectOptions{root: "."}
	var positional []string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--write":
			options.write = true
		case "--registry":
			if i+1 >= len(args) {
				return inspectOptions{}, fmt.Errorf("--registry requiere una ruta")
			}
			i++
			options.registryPath = args[i]
		case "-h", "--help":
			printInspectUsage()
			os.Exit(0)
		default:
			if len(args[i]) > 0 && args[i][0] == '-' {
				return inspectOptions{}, fmt.Errorf("opción desconocida: %s", args[i])
			}
			positional = append(positional, args[i])
		}
	}

	if len(positional) > 1 {
		return inspectOptions{}, fmt.Errorf("se esperaba una sola ruta de proyecto")
	}
	if len(positional) == 1 {
		options.root = positional[0]
	}

	return options, nil
}

func parseDoctorArgs(args []string) (doctorOptions, error) {
	options := doctorOptions{root: "."}
	var positional []string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--registry":
			if i+1 >= len(args) {
				return doctorOptions{}, fmt.Errorf("--registry requiere una ruta")
			}
			i++
			options.registryPath = args[i]
		case "-h", "--help":
			printDoctorUsage()
			os.Exit(0)
		default:
			if len(args[i]) > 0 && args[i][0] == '-' {
				return doctorOptions{}, fmt.Errorf("opción desconocida: %s", args[i])
			}
			positional = append(positional, args[i])
		}
	}

	if len(positional) > 1 {
		return doctorOptions{}, fmt.Errorf("se esperaba una sola ruta de proyecto")
	}
	if len(positional) == 1 {
		options.root = positional[0]
	}

	return options, nil
}


func resolveRegistryFile(name string) (string, error) {
	if home := os.Getenv("AI_ENGINEERING_OS_HOME"); home != "" {
		path := filepath.Join(home, "registry", name)
		if fileExists(path) {
			return path, nil
		}
	}

	local := filepath.Join("registry", name)
	if fileExists(local) {
		return local, nil
	}

	return "", fmt.Errorf("no se encontró registry/%s; ejecutá desde AI Engineering OS o configurá AI_ENGINEERING_OS_HOME", name)
}

func resolveSkillRegistry(explicit string) (string, error) {
	if explicit != "" {
		if !fileExists(explicit) {
			return "", fmt.Errorf("no existe el Skill Registry indicado: %s", explicit)
		}
		return explicit, nil
	}

	if home := os.Getenv("AI_ENGINEERING_OS_HOME"); home != "" {
		path := filepath.Join(home, "registry", "skills.yaml")
		if fileExists(path) {
			return path, nil
		}
	}

	local := filepath.Join("registry", "skills.yaml")
	if fileExists(local) {
		return local, nil
	}

	return "", fmt.Errorf("no se encontró el Skill Registry; usá --registry o configurá AI_ENGINEERING_OS_HOME")
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func printUsage() {
	fmt.Println("AI Engineering OS")
	fmt.Println()
	fmt.Println("Uso:")
	fmt.Println("  ai inspect [ruta-del-proyecto] [--registry ruta] [--write]")
	fmt.Println("  ai run init --task <tarea> --project <id> [--state-dir ruta]")
	fmt.Println("  ai run status <run-id> [--state-dir ruta] [--json]")
	fmt.Println("  ai run list [--state-dir ruta] [--json]")
	fmt.Println("  ai memory status")
	fmt.Println("  ai docs status")
	fmt.Println("  ai doctor [ruta-del-proyecto] [--registry ruta]")
	fmt.Println("  ai classify [--ai] <tarea>")
	fmt.Println("  ai plan [ruta-del-proyecto] --task <tarea> [--ai]")
	fmt.Println("  ai execute [ruta-del-proyecto] --task <tarea> (--dry-run | --runtime codex) [--ai]")
	fmt.Println("  ai propose [ruta-del-proyecto] --task <tarea> [--runtime auto|codex|cursor] [--model-policy auto|economy|balanced|deep] [--model id] [--ai]")
	fmt.Println("  ai stage [ruta-del-proyecto] --task <tarea> [--runtime auto|codex|cursor] [--model-policy auto|economy|balanced|deep] [--model id] [--ai]")
	fmt.Println("  ai diagnose cursor-proposal [project-path] --task <task> [--binary agent] [--response-file <file>|-] [--json]")
	fmt.Println("  ai plan [ruta-del-proyecto] [--intent implement|design|research|review|verify] --capability <id> [--capability <id>...]")
	fmt.Println("  ai gentle status")
	fmt.Println("  ai gentle plan <runtime>")
	fmt.Println("  ai gentle validate cursor")
	fmt.Println()
	fmt.Println("Comandos:")
	fmt.Println("  inspect   Detecta el stack y resuelve skills para un proyecto")
	fmt.Println("  run       Crea e inspecciona estado durable de ejecuciones")
	fmt.Println("  memory    Verifica el proveedor de memoria persistente")
	fmt.Println("  docs      Verifica el proveedor de documentación externa")
	fmt.Println("  doctor    Diagnostica el entorno local de AI Engineering OS")
	fmt.Println("  classify  Convierte una tarea natural en intent + capabilities; --ai habilita fallback Codex")
	fmt.Println("  plan      Resuelve agentes, skills y tools desde tarea o capabilities explícitas")
	fmt.Println("  execute   Ejecuta work units en dry-run o análisis read-only con un runtime")
	fmt.Println("  propose   Genera una propuesta estructurada y diff sin modificar el proyecto")
	fmt.Println("  stage     Aplica un patch aprobado únicamente en un workspace aislado")
	fmt.Println("  gentle    Descubre Gentle-AI y previsualiza proyección por runtime")
}

func printInspectUsage() {
	fmt.Println("Uso: ai inspect [ruta-del-proyecto] [--registry ruta] [--write]")
	fmt.Println()
	fmt.Println("Sin --write, el resultado se imprime en YAML y no modifica el proyecto.")
}

func printDoctorUsage() {
	fmt.Println("Uso: ai doctor [ruta-del-proyecto] [--registry ruta]")
	fmt.Println()
	fmt.Println("Ejecuta diagnósticos deterministas sin reparar ni modificar el entorno.")
}

func printPlanUsage() {
	fmt.Println("Uso:")
	fmt.Println("  ai plan [ruta-del-proyecto] --task <tarea> [--ai]")
	fmt.Println("  ai plan [ruta-del-proyecto] [--intent implement|design|research|review|verify] --capability <id> [--capability <id>...]")
	fmt.Println()
	fmt.Println("Con --task, intent y capabilities se clasifican primero. --ai habilita fallback Codex solo si hace falta.")
	fmt.Println("Con --capability, el intent por defecto es implement.")
	fmt.Println("Genera un plan determinista. No ejecuta agentes ni modifica el proyecto.")
}
