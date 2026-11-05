package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/controlplane"
)

type runCompactOptions struct {
	runID       string
	eventsDir   string
	snapshotDir string
	archiveDir  string
	json        bool
}

func runRunCompact(args []string) error {
	options, err := parseRunCompactArgs(args)
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
	archiveRoot, err := resolveRunAuxDir(
		options.archiveDir,
		"AI_ENGINEERING_OS_ARCHIVE_DIR",
		"control-plane-archives",
	)
	if err != nil {
		return err
	}
	archiveStore, err := controlplane.NewEventArchiveStore(archiveRoot)
	if err != nil {
		return err
	}
	result, err := controlplane.CompactEventHistory(
		context.Background(),
		eventStore,
		snapshotStore,
		archiveStore,
		options.runID,
		time.Now().UTC(),
	)
	if err != nil {
		return err
	}
	if options.json {
		return printJSON(result)
	}
	fmt.Println("Control Plane Event Compaction")
	fmt.Printf("Run ID: %s\n", result.RunID)
	fmt.Printf("Through event: #%d\n", result.ThroughSequence)
	fmt.Printf("Deleted individual files: %d\n", result.DeletedFiles)
	fmt.Printf("Archive digest: %s\n", result.ArchiveDigest)
	if result.AlreadyPresent {
		fmt.Println("Compacted history was already installed; verified cleanup was resumed.")
	}
	return nil
}

func parseRunCompactArgs(args []string) (runCompactOptions, error) {
	var options runCompactOptions
	var positional []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--events-dir":
			if i+1 >= len(args) {
				return runCompactOptions{}, fmt.Errorf("--events-dir requiere una ruta")
			}
			i++
			options.eventsDir = args[i]
		case "--snapshot-dir":
			if i+1 >= len(args) {
				return runCompactOptions{}, fmt.Errorf("--snapshot-dir requiere una ruta")
			}
			i++
			options.snapshotDir = args[i]
		case "--archive-dir":
			if i+1 >= len(args) {
				return runCompactOptions{}, fmt.Errorf("--archive-dir requiere una ruta")
			}
			i++
			options.archiveDir = args[i]
		case "--json":
			options.json = true
		case "-h", "--help":
			return runCompactOptions{}, fmt.Errorf("uso: ai run compact <run-id> [--events-dir ruta] [--snapshot-dir ruta] [--archive-dir ruta] [--json]")
		default:
			if strings.HasPrefix(args[i], "-") {
				return runCompactOptions{}, fmt.Errorf("opción desconocida: %s", args[i])
			}
			positional = append(positional, args[i])
		}
	}
	if len(positional) != 1 {
		return runCompactOptions{}, fmt.Errorf("uso: ai run compact <run-id>")
	}
	options.runID = positional[0]
	return options, nil
}
