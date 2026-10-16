package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/FernandoVillegas27/ai-engineering-os/core/project"
)

type inspectOptions struct {
	root         string
	registryPath string
	write        bool
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
	fmt.Println()
	fmt.Println("Comandos:")
	fmt.Println("  inspect   Detecta el stack y resuelve skills para un proyecto")
}

func printInspectUsage() {
	fmt.Println("Uso: ai inspect [ruta-del-proyecto] [--registry ruta] [--write]")
	fmt.Println()
	fmt.Println("Sin --write, el resultado se imprime en YAML y no modifica el proyecto.")
}
