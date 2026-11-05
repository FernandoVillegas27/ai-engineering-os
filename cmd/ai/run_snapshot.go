package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/controlplane"
)

type runSnapshotOptions struct {
	runID       string
	eventsDir   string
	snapshotDir string
	json        bool
}

func runRunSnapshot(args []string) error {
	options, err := parseRunSnapshotArgs(args)
	if err != nil {
		return err
	}
	eventStore, err := openControlPlaneStore(options.eventsDir)
	if err != nil {
		return err
	}
	snapshotRoot, err := resolveRunAuxDir(
		options.snapshotDir,
		"AI_ENGINEERING_OS_SNAPSHOT_DIR",
		"control-plane-snapshots",
	)
	if err != nil {
		return err
	}
	snapshotStore, err := controlplane.NewSnapshotStore(snapshotRoot)
	if err != nil {
		return err
	}
	record, err := snapshotStore.Create(
		context.Background(),
		eventStore,
		options.runID,
		time.Now().UTC(),
	)
	if err != nil {
		return err
	}
	if options.json {
		return printJSON(record)
	}
	fmt.Println("Control Plane Snapshot")
	fmt.Printf("Run ID: %s\n", record.RunID)
	fmt.Printf("Through event: #%d\n", record.ThroughSequence)
	fmt.Printf("Run status: %s\n", record.State.Status)
	fmt.Printf("Prefix digest: %s\n", record.PrefixDigest)
	return nil
}

func parseRunSnapshotArgs(args []string) (runSnapshotOptions, error) {
	var options runSnapshotOptions
	var positional []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--events-dir":
			if i+1 >= len(args) {
				return runSnapshotOptions{}, fmt.Errorf("--events-dir requiere una ruta")
			}
			i++
			options.eventsDir = args[i]
		case "--snapshot-dir":
			if i+1 >= len(args) {
				return runSnapshotOptions{}, fmt.Errorf("--snapshot-dir requiere una ruta")
			}
			i++
			options.snapshotDir = args[i]
		case "--json":
			options.json = true
		case "-h", "--help":
			return runSnapshotOptions{}, fmt.Errorf("uso: ai run snapshot <run-id> [--events-dir ruta] [--snapshot-dir ruta] [--json]")
		default:
			if strings.HasPrefix(args[i], "-") {
				return runSnapshotOptions{}, fmt.Errorf("opción desconocida: %s", args[i])
			}
			positional = append(positional, args[i])
		}
	}
	if len(positional) != 1 {
		return runSnapshotOptions{}, fmt.Errorf("uso: ai run snapshot <run-id>")
	}
	options.runID = positional[0]
	return options, nil
}
