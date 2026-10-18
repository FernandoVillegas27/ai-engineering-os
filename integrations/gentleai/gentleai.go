package gentleai

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/FernandoVillegas27/ai-engineering-os/core/projection"
)

const (
	ProviderID    = "gentle-ai"
	StableVersion = "v4.2.0"
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
	Binary string
	Runner Runner
}

func New() Client {
	return Client{
		Binary: "gentle-ai",
		Runner: OSRunner{},
	}
}

func (c Client) ID() string {
	return ProviderID
}

func (c Client) Status(ctx context.Context) (projection.Status, error) {
	binary := c.Binary
	if binary == "" {
		binary = "gentle-ai"
	}
	runner := c.Runner
	if runner == nil {
		runner = OSRunner{}
	}

	path, err := runner.LookPath(binary)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return projection.Status{
				Provider:  ProviderID,
				Available: false,
				Healthy:   false,
				Details:   "gentle-ai binary not found in PATH",
			}, nil
		}
		return projection.Status{}, fmt.Errorf("locate Gentle-AI binary: %w", err)
	}

	versionOutput, versionErr := runner.Run(ctx, path, "version")
	version := strings.TrimSpace(versionOutput)
	if versionErr != nil {
		return projection.Status{
			Provider:  ProviderID,
			Available: true,
			Healthy:   false,
			Details:   fmt.Sprintf("gentle-ai version probe failed: %v", versionErr),
		}, nil
	}

	doctorOutput, doctorErr := runner.Run(ctx, path, "doctor")
	details := strings.TrimSpace(doctorOutput)
	if doctorErr != nil && details == "" {
		details = doctorErr.Error()
	}

	healthy := doctorErr == nil && doctorReportsHealthy(details)

	return projection.Status{
		Provider:  ProviderID,
		Available: true,
		Healthy:   healthy,
		Version:   version,
		Details:   details,
	}, nil
}

func doctorReportsHealthy(output string) bool {
	normalized := strings.ToLower(strings.TrimSpace(output))

	if strings.Contains(normalized, "status:") {
		if strings.Contains(normalized, "status:  unhealthy") ||
			strings.Contains(normalized, "status: unhealthy") {
			return false
		}
		if strings.Contains(normalized, "status:  healthy") ||
			strings.Contains(normalized, "status: healthy") {
			return true
		}
	}

	return !strings.Contains(normalized, "[xx]") &&
		!strings.Contains(normalized, " failed") &&
		!strings.Contains(normalized, "unhealthy")
}

func (c Client) PreviewInstall(target string) (projection.Plan, error) {
	target, err := normalizeTarget(target)
	if err != nil {
		return projection.Plan{}, err
	}

	return projection.Plan{
		Provider: ProviderID,
		Target:   target,
		Mode:     "install-preview",
		Command:  binaryName(c.Binary),
		Args: []string{
			"install",
			"--dry-run",
			"--agent",
			target,
			"--preset",
			"minimal",
		},
		Mutates: false,
	}, nil
}

func (c Client) PreviewSync(target string) (projection.Plan, error) {
	target, err := normalizeTarget(target)
	if err != nil {
		return projection.Plan{}, err
	}

	return projection.Plan{
		Provider: ProviderID,
		Target:   target,
		Mode:     "sync-preview",
		Command:  binaryName(c.Binary),
		Args: []string{
			"sync",
			"--dry-run",
			"--agent",
			target,
		},
		Mutates: false,
	}, nil
}

func SupportedTargets() []string {
	return []string{
		"pi",
		"opencode",
		"claude-code",
		"codex",
		"cursor",
		"vscode-copilot",
		"gemini-cli",
		"kilocode",
		"kimi",
		"kiro-ide",
		"qwen-code",
		"hermes",
		"antigravity",
		"windsurf",
		"openclaw",
		"trae-ide",
		"conductor",
	}
}

func InstallCommandWindows() []string {
	return []string{
		"go",
		"install",
		"github.com/gentleman-programming/gentle-ai/v4/cmd/gentle-ai@" + StableVersion,
	}
}

func normalizeTarget(target string) (string, error) {
	target = strings.TrimSpace(target)
	for _, supported := range SupportedTargets() {
		if target == supported {
			return target, nil
		}
	}
	return "", fmt.Errorf("unsupported Gentle-AI target %q", target)
}

func binaryName(binary string) string {
	if strings.TrimSpace(binary) == "" {
		return "gentle-ai"
	}
	return binary
}
