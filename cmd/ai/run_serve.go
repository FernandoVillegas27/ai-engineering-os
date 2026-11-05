package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/artifact"
	"github.com/FernandoVillegas27/ai-engineering-os/core/controlplane"
	"github.com/FernandoVillegas27/ai-engineering-os/core/message"
	"github.com/FernandoVillegas27/ai-engineering-os/core/promotioncandidate"
	"github.com/FernandoVillegas27/ai-engineering-os/core/runcheckpoint"
	"github.com/FernandoVillegas27/ai-engineering-os/core/runcontrol"
	"github.com/FernandoVillegas27/ai-engineering-os/core/runstate"
)

type runServeOptions struct {
	addr         string
	eventsDir    string
	artifactsDir string
	messagesDir  string
	promotionDir string
	stateDir     string
	checkpointDir      string
	snapshotDir        string
	controlDir         string
	recoveryStaleAfter time.Duration
}

func runRunServe(args []string) error {
	options, err := parseRunServeArgs(args)
	if err != nil {
		return err
	}
	if err := validateLoopbackAddr(options.addr); err != nil {
		return err
	}
	store, err := openControlPlaneStore(options.eventsDir)
	if err != nil {
		return err
	}
	artifactsRoot, err := resolveRunAuxDir(options.artifactsDir, "AI_ENGINEERING_OS_ARTIFACTS_DIR", "artifacts")
	if err != nil {
		return err
	}
	artifactStore, err := artifact.NewStore(artifactsRoot)
	if err != nil {
		return err
	}
	messagesRoot, err := resolveRunAuxDir(options.messagesDir, "AI_ENGINEERING_OS_MESSAGES_DIR", "messages")
	if err != nil {
		return err
	}
	messageBus, err := message.NewBus(messagesRoot)
	if err != nil {
		return err
	}
	promotionRoot, err := resolveRunAuxDir(options.promotionDir, "AI_ENGINEERING_OS_PROMOTION_DIR", "promotion-candidates")
	if err != nil {
		return err
	}
	promotionStore, err := promotioncandidate.NewStore(promotionRoot)
	if err != nil {
		return err
	}
	runStore, err := openRunStore(options.stateDir)
	if err != nil {
		return err
	}
	checkpointRoot, err := resolveRunAuxDir(
		options.checkpointDir,
		"AI_ENGINEERING_OS_CHECKPOINT_DIR",
		"run-checkpoints",
	)
	if err != nil {
		return err
	}
	checkpointStore, err := runcheckpoint.NewStore(runcheckpoint.Config{Root: checkpointRoot})
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
	controlStore, err := openRunControlStore(options.controlDir)
	if err != nil {
		return err
	}
	controlsProvider := func(ctx context.Context, runID string) (controlplane.RunControls, error) {
		run, err := runStore.Load(runID)
		if err != nil {
			return controlplane.RunControls{}, err
		}
		state, _, err := snapshotStore.ProjectVerified(ctx, store, runID)
		if err != nil {
			return controlplane.RunControls{}, err
		}
		checkpointAvailable := false
		if state.Pipeline.CheckpointPhase != "" {
			phase := runcheckpoint.Phase(state.Pipeline.CheckpointPhase)
			if err := checkpointStore.Check(ctx, runID, phase); err == nil {
				checkpointAvailable = true
			}
		}
		leaseStale := false
		if run.Status == runstate.StatusRunning && state.Status == controlplane.RunRunning {
			stale, _, leaseErr := controlStore.LeaseStale(
				runID,
				time.Now().UTC(),
				options.recoveryStaleAfter,
			)
			if leaseErr == nil {
				leaseStale = stale
			} else if !os.IsNotExist(leaseErr) {
				return controlplane.RunControls{}, leaseErr
			}
		}
		return runcontrol.DeriveCapabilities(runcontrol.CapabilityInput{
			Run:                 run,
			ControlPlane:        state,
			CheckpointAvailable: checkpointAvailable,
			LeaseStale:          leaseStale,
		}), nil
	}

	server := &http.Server{
		Addr: options.addr,
		Handler: (controlplane.HTTPServer{
			Store:     store,
			Artifacts: artifactStore,
			Messages:   messageBus,
			Promotions: promotionStore,
			Snapshots:  snapshotStore,
			Controls:   controlsProvider,
		}).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	fmt.Printf("Control Plane live API: http://%s\n", options.addr)
	fmt.Println("Read-only endpoints:")
	fmt.Println("  GET /health")
	fmt.Println("  GET /runs/<run-id>/state")
	fmt.Println("  GET /runs/<run-id>/events")
	fmt.Println("  GET /runs/<run-id>/stream?after=<sequence>")
	fmt.Println("  GET /runs/<run-id>/messages")
	fmt.Println("  GET /runs/<run-id>/artifacts")
	fmt.Println("  GET /runs/<run-id>/promotion")
	fmt.Println("  GET /runs/<run-id>/controls")
	fmt.Println("Binding is restricted to loopback in this version.")

	err = server.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func parseRunServeArgs(args []string) (runServeOptions, error) {
	options := runServeOptions{addr: "127.0.0.1:8765", recoveryStaleAfter: 30 * time.Second}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--addr":
			if i+1 >= len(args) {
				return runServeOptions{}, fmt.Errorf("--addr requiere host:puerto")
			}
			i++
			options.addr = args[i]
		case "--events-dir":
			if i+1 >= len(args) {
				return runServeOptions{}, fmt.Errorf("--events-dir requiere una ruta")
			}
			i++
			options.eventsDir = args[i]
		case "--artifacts-dir":
			if i+1 >= len(args) {
				return runServeOptions{}, fmt.Errorf("--artifacts-dir requiere una ruta")
			}
			i++
			options.artifactsDir = args[i]
		case "--messages-dir":
			if i+1 >= len(args) {
				return runServeOptions{}, fmt.Errorf("--messages-dir requiere una ruta")
			}
			i++
			options.messagesDir = args[i]
		case "--promotion-dir":
			if i+1 >= len(args) {
				return runServeOptions{}, fmt.Errorf("--promotion-dir requiere una ruta")
			}
			i++
			options.promotionDir = args[i]
		case "--state-dir":
			if i+1 >= len(args) {
				return runServeOptions{}, fmt.Errorf("--state-dir requiere una ruta")
			}
			i++
			options.stateDir = args[i]
		case "--checkpoint-dir":
			if i+1 >= len(args) {
				return runServeOptions{}, fmt.Errorf("--checkpoint-dir requiere una ruta")
			}
			i++
			options.checkpointDir = args[i]
		case "--snapshot-dir":
			if i+1 >= len(args) {
				return runServeOptions{}, fmt.Errorf("--snapshot-dir requiere una ruta")
			}
			i++
			options.snapshotDir = args[i]
		case "--control-dir":
			if i+1 >= len(args) {
				return runServeOptions{}, fmt.Errorf("--control-dir requiere una ruta")
			}
			i++
			options.controlDir = args[i]
		case "--recovery-stale-after":
			if i+1 >= len(args) {
				return runServeOptions{}, fmt.Errorf("--recovery-stale-after requiere una duración")
			}
			i++
			value, err := time.ParseDuration(args[i])
			if err != nil || value < 5*time.Second {
				return runServeOptions{}, fmt.Errorf("--recovery-stale-after debe ser una duración >= 5s")
			}
			options.recoveryStaleAfter = value
		case "-h", "--help":
			return runServeOptions{}, fmt.Errorf("uso: ai run serve [--addr 127.0.0.1:8765] [--events-dir ruta] [--artifacts-dir ruta] [--messages-dir ruta] [--promotion-dir ruta] [--state-dir ruta] [--checkpoint-dir ruta] [--snapshot-dir ruta] [--control-dir ruta] [--recovery-stale-after 30s]")
		default:
			return runServeOptions{}, fmt.Errorf("opción desconocida: %s", args[i])
		}
	}
	return options, nil
}

func validateLoopbackAddr(addr string) error {
	host, port, err := net.SplitHostPort(strings.TrimSpace(addr))
	if err != nil {
		return fmt.Errorf("--addr debe tener formato host:puerto: %w", err)
	}
	if strings.TrimSpace(host) == "" {
		return fmt.Errorf("--addr debe indicar un host loopback explícito")
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return fmt.Errorf("--addr debe usar un puerto entre 1 y 65535")
	}

	if strings.EqualFold(host, "localhost") {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("por seguridad, --addr solo puede usar localhost o una IP loopback")
	}
	return nil
}
