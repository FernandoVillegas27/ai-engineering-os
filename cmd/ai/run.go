package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/runstate"
)

type runCommandOptions struct {
	stateDir string
	json     bool
}

func runRun(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("uso: ai run <init|status|list|transition> ...")
	}
	switch args[0] {
	case "init":
		return runRunInit(args[1:])
	case "status":
		return runRunStatus(args[1:])
	case "list":
		return runRunList(args[1:])
	case "transition":
		return runRunTransition(args[1:])
	default:
		return fmt.Errorf("subcomando run desconocido: %s", args[0])
	}
}

func runRunInit(args []string) error {
	options := runCommandOptions{}
	var task, projectID, projectRoot string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--state-dir":
			if i+1 >= len(args) {
				return fmt.Errorf("--state-dir requiere una ruta")
			}
			i++
			options.stateDir = args[i]
		case "--task":
			if i+1 >= len(args) {
				return fmt.Errorf("--task requiere texto")
			}
			i++
			task = args[i]
		case "--project":
			if i+1 >= len(args) {
				return fmt.Errorf("--project requiere un id")
			}
			i++
			projectID = args[i]
		case "--project-root":
			if i+1 >= len(args) {
				return fmt.Errorf("--project-root requiere una ruta")
			}
			i++
			projectRoot = args[i]
		case "--json":
			options.json = true
		case "-h", "--help":
			printRunUsage()
			return nil
		default:
			return fmt.Errorf("opción desconocida: %s", args[i])
		}
	}
	if strings.TrimSpace(task) == "" || strings.TrimSpace(projectID) == "" {
		return fmt.Errorf("--task y --project son obligatorios")
	}
	if strings.TrimSpace(projectRoot) != "" {
		absolute, err := filepath.Abs(projectRoot)
		if err != nil {
			return fmt.Errorf("resolver project root: %w", err)
		}
		projectRoot = absolute
	}

	store, err := openRunStore(options.stateDir)
	if err != nil {
		return err
	}
	run, err := runstate.New(runstate.NewRunRequest{
		Task:        task,
		ProjectID:   projectID,
		ProjectRoot: projectRoot,
	}, time.Now().UTC())
	if err != nil {
		return err
	}
	if err := store.Save(run); err != nil {
		return err
	}
	return printRunRecord(run, options.json)
}

func runRunStatus(args []string) error {
	options := runCommandOptions{}
	var id string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--state-dir":
			if i+1 >= len(args) {
				return fmt.Errorf("--state-dir requiere una ruta")
			}
			i++
			options.stateDir = args[i]
		case "--json":
			options.json = true
		case "-h", "--help":
			printRunUsage()
			return nil
		default:
			if strings.HasPrefix(args[i], "-") {
				return fmt.Errorf("opción desconocida: %s", args[i])
			}
			if id != "" {
				return fmt.Errorf("se esperaba un solo run id")
			}
			id = args[i]
		}
	}
	if id == "" {
		return fmt.Errorf("run id es obligatorio")
	}
	store, err := openRunStore(options.stateDir)
	if err != nil {
		return err
	}
	run, err := store.Load(id)
	if err != nil {
		return err
	}
	return printRunRecord(run, options.json)
}

func runRunList(args []string) error {
	options := runCommandOptions{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--state-dir":
			if i+1 >= len(args) {
				return fmt.Errorf("--state-dir requiere una ruta")
			}
			i++
			options.stateDir = args[i]
		case "--json":
			options.json = true
		case "-h", "--help":
			printRunUsage()
			return nil
		default:
			return fmt.Errorf("opción desconocida: %s", args[i])
		}
	}
	store, err := openRunStore(options.stateDir)
	if err != nil {
		return err
	}
	runs, err := store.List()
	if err != nil {
		return err
	}
	if options.json {
		return printJSON(runs)
	}
	if len(runs) == 0 {
		fmt.Println("No durable runs found.")
		return nil
	}
	for _, run := range runs {
		fmt.Printf("%s  %-10s  %s  project=%s\n", run.ID, run.Status, run.Task, run.ProjectID)
	}
	return nil
}

func runRunTransition(args []string) error {
	options := runCommandOptions{}
	var id string
	var status runstate.Status
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--state-dir":
			if i+1 >= len(args) {
				return fmt.Errorf("--state-dir requiere una ruta")
			}
			i++
			options.stateDir = args[i]
		case "--status":
			if i+1 >= len(args) {
				return fmt.Errorf("--status requiere un valor")
			}
			i++
			status = runstate.Status(args[i])
		case "--json":
			options.json = true
		case "-h", "--help":
			printRunUsage()
			return nil
		default:
			if strings.HasPrefix(args[i], "-") {
				return fmt.Errorf("opción desconocida: %s", args[i])
			}
			if id != "" {
				return fmt.Errorf("se esperaba un solo run id")
			}
			id = args[i]
		}
	}
	if id == "" || status == "" {
		return fmt.Errorf("run id y --status son obligatorios")
	}
	store, err := openRunStore(options.stateDir)
	if err != nil {
		return err
	}
	run, err := store.Load(id)
	if err != nil {
		return err
	}
	if err := runstate.Transition(&run, status, time.Now().UTC()); err != nil {
		return err
	}
	if err := store.Save(run); err != nil {
		return err
	}
	return printRunRecord(run, options.json)
}

func openRunStore(explicit string) (*runstate.Store, error) {
	root, err := resolveRunStateDir(explicit)
	if err != nil {
		return nil, err
	}
	return runstate.NewStore(root)
}

func resolveRunStateDir(explicit string) (string, error) {
	if strings.TrimSpace(explicit) != "" {
		return explicit, nil
	}
	if value := strings.TrimSpace(os.Getenv("AI_ENGINEERING_OS_STATE_DIR")); value != "" {
		return value, nil
	}
	configRoot, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolver directorio de configuración: %w", err)
	}
	return filepath.Join(configRoot, "ai-engineering-os", "runs"), nil
}

func printRunRecord(run runstate.Run, asJSON bool) error {
	if asJSON {
		return printJSON(run)
	}
	fmt.Println("Durable Run")
	fmt.Printf("ID: %s\n", run.ID)
	fmt.Printf("Task ID: %s\n", run.TaskID)
	fmt.Printf("Status: %s\n", run.Status)
	fmt.Printf("Project: %s\n", run.ProjectID)
	fmt.Printf("Task: %s\n", run.Task)
	fmt.Printf("Updated: %s\n", run.UpdatedAt.Format(time.RFC3339))
	if len(run.WorkUnits) > 0 {
		fmt.Println("Agents:")
		for _, unit := range run.WorkUnits {
			fmt.Printf("  - %s [%s] %s\n", unit.AgentID, unit.Status, unit.ID)
		}
	}
	if run.Quality != nil {
		fmt.Printf("Quality: passed=%t reason=%s\n", run.Quality.Passed, run.Quality.Reason)
	}
	return nil
}

func printJSON(value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

func printRunUsage() {
	fmt.Println("Uso:")
	fmt.Println("  ai run init --task <tarea> --project <id> [--project-root ruta] [--state-dir ruta] [--json]")
	fmt.Println("  ai run status <run-id> [--state-dir ruta] [--json]")
	fmt.Println("  ai run list [--state-dir ruta] [--json]")
	fmt.Println("  ai run transition <run-id> --status planned|running|paused|completed|failed|cancelled [--state-dir ruta] [--json]")
	fmt.Println()
	fmt.Println("Sin --state-dir usa AI_ENGINEERING_OS_STATE_DIR o el directorio de configuración del usuario.")
}
