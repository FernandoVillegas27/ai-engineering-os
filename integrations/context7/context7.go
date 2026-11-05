package context7

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/docprovider"
	"github.com/FernandoVillegas27/ai-engineering-os/core/toolhealth"
)

const (
	ProviderID     = "context7"
	DefaultPackage = "@upstash/context7-mcp@4.4.2"
)

type Runner interface {
	LookPath(file string) (string, error)
	Run(ctx context.Context, name string, args ...string) (string, error)
}

type OSRunner struct{}

func (OSRunner) LookPath(file string) (string, error) {
	return exec.LookPath(file)
}

func (OSRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	command := exec.CommandContext(ctx, name, args...)
	output, err := command.CombinedOutput()
	return strings.TrimSpace(string(output)), err
}

type Client struct {
	NPX          string
	Package      string
	Runner       Runner
	ProbeTimeout time.Duration
}

type MCPConfig struct {
	Command string   `json:"command" yaml:"command"`
	Args    []string `json:"args" yaml:"args"`
}

func New() Client {
	return Client{
		NPX:          "npx",
		Package:      DefaultPackage,
		Runner:       OSRunner{},
		ProbeTimeout: toolhealth.DefaultProbeTimeout,
	}
}

func (c Client) ID() string {
	return ProviderID
}

func (c Client) Health(ctx context.Context) toolhealth.Status {
	npx := c.NPX
	if npx == "" {
		npx = "npx"
	}
	runner := c.Runner
	if runner == nil {
		runner = OSRunner{}
	}
	path, err := runner.LookPath(npx)
	if err != nil {
		failure := toolhealth.Classify(err, "")
		if errors.Is(err, exec.ErrNotFound) {
			failure.Kind = toolhealth.FailureUnavailable
			failure.Detail = "npx not found in PATH"
		}
		return toolhealth.Status{
			Provider: ProviderID, Available: false, Healthy: false,
			Details: failure.Detail, Failure: failure,
		}
	}
	timeout := c.ProbeTimeout
	if timeout <= 0 {
		timeout = toolhealth.DefaultProbeTimeout
	}
	version, failure, duration := toolhealth.Run(ctx, timeout, func(runCtx context.Context) (string, error) {
		return runner.Run(runCtx, path, "--version")
	})
	if failure.Kind != toolhealth.FailureNone {
		return toolhealth.Status{
			Provider: ProviderID, Available: true, Healthy: false,
			Details: failure.Detail, Failure: failure, Duration: duration,
		}
	}
	return toolhealth.Status{
		Provider: ProviderID, Available: true, Healthy: true,
		Version: strings.TrimSpace(version),
		Details: "local MCP launcher is available; remote MCP connectivity remains host-verified",
		Duration: duration,
	}
}

func (c Client) Status(ctx context.Context) (docprovider.Status, error) {
	health := c.Health(ctx)
	return docprovider.Status{
		Provider: health.Provider,
		Available: health.Available,
		Ready: health.Healthy,
		Version: health.Version,
		Details: health.Details,
	}, nil
}

func (c Client) MCP() MCPConfig {
	npx := c.NPX
	if npx == "" {
		npx = "npx"
	}
	pkg := c.Package
	if pkg == "" {
		pkg = DefaultPackage
	}

	return MCPConfig{
		Command: npx,
		Args: []string{
			"-y",
			pkg,
		},
	}
}

func RecommendedOperations() []string {
	return []string{
		"resolve-library-id",
		"query-docs",
	}
}
