package context7

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/FernandoVillegas27/ai-engineering-os/core/documentation"
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
	NPX     string
	Package string
	Runner  Runner
}

type MCPConfig struct {
	Command string   `json:"command" yaml:"command"`
	Args    []string `json:"args" yaml:"args"`
}

func New() Client {
	return Client{
		NPX:     "npx",
		Package: DefaultPackage,
		Runner:  OSRunner{},
	}
}

func (c Client) ID() string {
	return ProviderID
}

func (c Client) Status(ctx context.Context) (documentation.Status, error) {
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
		if errors.Is(err, exec.ErrNotFound) {
			return documentation.Status{
				Provider:  ProviderID,
				Available: false,
				Ready:     false,
				Details:   "npx not found in PATH",
			}, nil
		}
		return documentation.Status{}, fmt.Errorf("locate npx: %w", err)
	}

	version, versionErr := runner.Run(ctx, path, "--version")
	if versionErr != nil {
		return documentation.Status{
			Provider:  ProviderID,
			Available: true,
			Ready:     false,
			Details:   fmt.Sprintf("npx is available but could not report a version: %v", versionErr),
		}, nil
	}

	return documentation.Status{
		Provider:  ProviderID,
		Available: true,
		Ready:     true,
		Version:   strings.TrimSpace(version),
		Details:   "local MCP launcher is available; server connectivity is verified by the MCP host",
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
