package main

import (
	"fmt"

	"github.com/FernandoVillegas27/ai-engineering-os/core/buildinfo"
)

func runVersion(args []string) error {
	asJSON := false
	for _, arg := range args {
		switch arg {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Println("Uso: ai version [--json]")
			return nil
		default:
			return fmt.Errorf("opción desconocida: %s", arg)
		}
	}
	info := buildinfo.Current()
	if asJSON {
		return printJSON(info)
	}
	fmt.Printf("AI Engineering OS %s\n", info.Version)
	fmt.Printf("Commit: %s\n", info.Commit)
	fmt.Printf("Built: %s\n", info.Date)
	fmt.Printf("Go: %s\n", info.GoVersion)
	fmt.Printf("Platform: %s/%s\n", info.OS, info.Arch)
	if info.Modified {
		fmt.Println("Working tree at build time: modified")
	}
	return nil
}
