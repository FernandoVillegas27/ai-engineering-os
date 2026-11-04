package main

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/controlplane"
)

type runServeOptions struct {
	addr      string
	eventsDir string
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

	server := &http.Server{
		Addr:              options.addr,
		Handler:           (controlplane.HTTPServer{Store: store}).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	fmt.Printf("Control Plane live API: http://%s\n", options.addr)
	fmt.Println("Read-only endpoints:")
	fmt.Println("  GET /health")
	fmt.Println("  GET /runs/<run-id>/state")
	fmt.Println("  GET /runs/<run-id>/events")
	fmt.Println("  GET /runs/<run-id>/stream?after=<sequence>")
	fmt.Println("Binding is restricted to loopback in this version.")

	err = server.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func parseRunServeArgs(args []string) (runServeOptions, error) {
	options := runServeOptions{addr: "127.0.0.1:8765"}
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
		case "-h", "--help":
			return runServeOptions{}, fmt.Errorf("uso: ai run serve [--addr 127.0.0.1:8765] [--events-dir ruta]")
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
