package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/FernandoVillegas27/ai-engineering-os/core/project"
)

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
	flags := flag.NewFlagSet("inspect", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)

	registryPath := flags.String("registry", "", "Ruta al registry/skills.yaml")
	write := flags.Bool("write", false, "Escribir el resultado en .ai/project.yaml")

	flags.Usage = func() {
		fmt.Fprintln(os.Stderr, "Uso: ai inspect [ruta-del-proyecto] [--registry ruta] [--write]")
	}

	if err := flags.Parse(args); err != nil {
		return err
	}

	root := "."
	if flags.NArg() > 1 {
		return fmt.Errorf("se esperaba una sola ruta de proyecto")
	}
	if flags.NArg() == 1 {
		root = flags.Arg(0)
	}

	resolvedRegistry, err := resolveSkillRegistry(*registryPath)
	if err != nil {
		return err
	}

	context, err := project.Build(root, resolvedRegistry)
	if err != nil {
		return err
	}

	if *write {
		outputPath, err := project.Write(root, context)
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

func resolveSkillRegistry(explicit string) (string, error) {
	if explicit != "" {
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
