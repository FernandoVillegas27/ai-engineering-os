package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/controlplane"
)

type runArchiveOptions struct {
	runID       string
	eventsDir   string
	snapshotDir string
	archiveDir  string
	json        bool
}

func runRunArchive(args []string) error {
	options, err := parseRunArchiveArgs(args)
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
	record, err := archiveStore.Create(
		context.Background(),
		eventStore,
		snapshotStore,
		options.runID,
		time.Now().UTC(),
	)
	if err != nil {
		return err
	}
	if options.json {
		return printJSON(record)
	}
	fmt.Println("Control Plane Event Archive")
	fmt.Printf("Run ID: %s\n", record.RunID)
	fmt.Printf("Through event: #%d\n", record.ThroughSequence)
	fmt.Printf("Events archived: %d\n", len(record.Entries))
	fmt.Printf("Prefix digest: %s\n", record.PrefixDigest)
	fmt.Printf("Archive digest: %s\n", record.RecordDigest)
	fmt.Println("Live event files were preserved.")
	return nil
}

func parseRunArchiveArgs(args []string) (runArchiveOptions, error) {
	var options runArchiveOptions
	var positional []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--events-dir":
			if i+1 >= len(args) {
				return runArchiveOptions{}, fmt.Errorf("--events-dir requiere una ruta")
			}
			i++
			options.eventsDir = args[i]
		case "--snapshot-dir":
			if i+1 >= len(args) {
				return runArchiveOptions{}, fmt.Errorf("--snapshot-dir requiere una ruta")
			}
			i++
			options.snapshotDir = args[i]
		case "--archive-dir":
			if i+1 >= len(args) {
				return runArchiveOptions{}, fmt.Errorf("--archive-dir requiere una ruta")
			}
			i++
			options.archiveDir = args[i]
		case "--json":
			options.json = true
		case "-h", "--help":
			return runArchiveOptions{}, fmt.Errorf("uso: ai run archive <run-id> [--events-dir ruta] [--snapshot-dir ruta] [--archive-dir ruta] [--json]")
		default:
			if strings.HasPrefix(args[i], "-") {
				return runArchiveOptions{}, fmt.Errorf("opción desconocida: %s", args[i])
			}
			positional = append(positional, args[i])
		}
	}
	if len(positional) != 1 {
		return runArchiveOptions{}, fmt.Errorf("uso: ai run archive <run-id>")
	}
	options.runID = positional[0]
	return options, nil
}
