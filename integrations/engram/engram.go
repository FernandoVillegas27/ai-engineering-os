package engram

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/FernandoVillegas27/ai-engineering-os/core/memory"
)

const ProviderID = "engram"

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
	Binary string
	Runner Runner
}

type MCPConfig struct {
	Command string   `json:"command" yaml:"command"`
	Args    []string `json:"args" yaml:"args"`
}

func New() Client {
	return Client{
		Binary: "engram",
		Runner: OSRunner{},
	}
}

func (c Client) ID() string {
	return ProviderID
}

func (c Client) Status(ctx context.Context) (memory.Status, error) {
	binary := c.Binary
	if binary == "" {
		binary = "engram"
	}
	runner := c.Runner
	if runner == nil {
		runner = OSRunner{}
	}

	path, err := runner.LookPath(binary)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return memory.Status{
				Provider:  ProviderID,
				Available: false,
				Healthy:   false,
				Details:   "engram binary not found in PATH",
			}, nil
		}
		return memory.Status{}, fmt.Errorf("locate Engram binary: %w", err)
	}

	versionOutput, versionErr := runner.Run(ctx, path, "version")
	version := strings.TrimSpace(versionOutput)
	if versionErr != nil {
		version = ""
	}

	doctorOutput, doctorErr := runner.Run(ctx, path, "doctor", "--json")
	details := strings.TrimSpace(doctorOutput)
	if details == "" && doctorErr != nil {
		details = doctorErr.Error()
	}

	return memory.Status{
		Provider:  ProviderID,
		Available: true,
		Healthy:   doctorErr == nil,
		Version:   version,
		Details:   details,
	}, nil
}

func (c Client) MCP() MCPConfig {
	binary := c.Binary
	if binary == "" {
		binary = "engram"
	}

	return MCPConfig{
		Command: binary,
		Args:    []string{"mcp", "--tools=agent"},
	}
}

func SetupCommand(runtime string) ([]string, error) {
	targets := map[string]string{
		"claude-code":     "claude-code",
		"pi":              "pi",
		"opencode":        "opencode",
		"gemini-cli":      "gemini-cli",
		"codex":           "codex",
		"antigravity-cli": "antigravity-cli",
		"windsurf":        "windsurf",
		"qwen":            "qwen",
		"kiro":            "kiro",
		"cursor":          "cursor",
		"vscode-copilot":  "vscode-copilot",
		"kilocode":        "kilocode",
		"kimi":            "kimi",
		"commandcode":     "commandcode",
	}

	target, ok := targets[runtime]
	if !ok {
		return nil, fmt.Errorf("unsupported Engram setup target %q", runtime)
	}

	return []string{"setup", target}, nil
}
