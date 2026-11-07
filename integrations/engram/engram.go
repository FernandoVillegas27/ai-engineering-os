package engram

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/memory"
	"github.com/FernandoVillegas27/ai-engineering-os/core/toolhealth"
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
	Binary           string
	Runner           Runner
	ProbeTimeout     time.Duration
	OperationTimeout time.Duration
}

type MCPConfig struct {
	Command string   `json:"command" yaml:"command"`
	Args    []string `json:"args" yaml:"args"`
}

func New() Client {
	return Client{
		Binary:           "engram",
		Runner:           OSRunner{},
		ProbeTimeout:     toolhealth.DefaultProbeTimeout,
		OperationTimeout: toolhealth.DefaultOperationTimeout,
	}
}

func (c Client) ID() string {
	return ProviderID
}

func (c Client) Health(ctx context.Context) toolhealth.Status {
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
		failure := toolhealth.Classify(err, "")
		if errors.Is(err, exec.ErrNotFound) {
			failure.Kind = toolhealth.FailureUnavailable
			failure.Detail = "engram binary not found in PATH"
		}
		return toolhealth.Status{
			Provider: ProviderID, Available: false, Healthy: false,
			Details: failure.Detail, Failure: failure,
		}
	}

	probeTimeout := c.ProbeTimeout
	if probeTimeout <= 0 {
		probeTimeout = toolhealth.DefaultProbeTimeout
	}
	versionOutput, versionFailure, versionDuration := toolhealth.Run(
		ctx, probeTimeout,
		func(runCtx context.Context) (string, error) {
			return runner.Run(runCtx, path, "version")
		},
	)
	version := strings.TrimSpace(versionOutput)

	doctorOutput, doctorFailure, doctorDuration := toolhealth.Run(
		ctx, probeTimeout,
		func(runCtx context.Context) (string, error) {
			return runner.Run(runCtx, path, "doctor", "--json")
		},
	)
	details := strings.TrimSpace(doctorOutput)
	if doctorFailure.Kind != toolhealth.FailureNone && details == "" {
		details = doctorFailure.Detail
	}
	failure := doctorFailure
	if failure.Kind == toolhealth.FailureNone && versionFailure.Kind != toolhealth.FailureNone {
		failure = versionFailure
	}
	healthy := doctorFailure.Kind == toolhealth.FailureNone
	return toolhealth.Status{
		Provider: ProviderID,
		Available: true,
		Healthy: healthy,
		Version: version,
		Details: details,
		Failure: failure,
		Duration: versionDuration + doctorDuration,
	}
}

func (c Client) Status(ctx context.Context) (memory.Status, error) {
	health := c.Health(ctx)
	return memory.Status{
		Provider:  health.Provider,
		Available: health.Available,
		Healthy:   health.Healthy,
		Version:   health.Version,
		Details:   health.Details,
	}, nil
}

func (c Client) Save(ctx context.Context, request memory.SaveRequest) (memory.SaveResult, error) {
	if err := memory.ValidateSaveRequest(request); err != nil {
		return memory.SaveResult{}, err
	}
	path, runner, err := c.resolve()
	if err != nil {
		return memory.SaveResult{}, err
	}

	args := []string{
		"save",
		strings.TrimSpace(request.Title),
		strings.TrimSpace(request.Content),
		"--project", strings.TrimSpace(request.Project),
		"--type", string(request.Type),
		"--scope", "project",
	}
	if topic := strings.TrimSpace(request.TopicKey); topic != "" {
		args = append(args, "--topic", topic)
	}
	timeout := c.OperationTimeout
	if timeout <= 0 {
		timeout = toolhealth.DefaultOperationTimeout
	}
	output, failure, _ := toolhealth.Run(ctx, timeout, func(runCtx context.Context) (string, error) {
		return runner.Run(runCtx, path, args...)
	})
	if failure.Kind != toolhealth.FailureNone {
		return memory.SaveResult{}, fmt.Errorf("save Engram memory [%s]: %s", failure.Kind, failure.Detail)
	}
	return memory.SaveResult{
		Provider: ProviderID,
		Project:  strings.TrimSpace(request.Project),
		Saved:    true,
		Summary:  strings.TrimSpace(output),
		MemoryID: parseSavedMemoryID(output),
	}, nil
}

var savedMemoryIDPattern = regexp.MustCompile(`(?i)Memory saved:\s*#([0-9]+)`)

func parseSavedMemoryID(output string) string {
	match := savedMemoryIDPattern.FindStringSubmatch(output)
	if len(match) != 2 {
		return ""
	}
	return match[1]
}

func (c Client) Delete(ctx context.Context, request memory.DeleteRequest) (memory.DeleteResult, error) {
	if err := memory.ValidateDeleteRequest(request); err != nil {
		return memory.DeleteResult{}, err
	}
	path, runner, err := c.resolve()
	if err != nil {
		return memory.DeleteResult{}, err
	}
	timeout := c.OperationTimeout
	if timeout <= 0 {
		timeout = toolhealth.DefaultOperationTimeout
	}

	// Engram's CLI delete command is ID-based. Verify that the observation
	// belongs to the caller-supplied project first; project ownership is
	// immutable, so a successful scoped lookup establishes the delete target.
	_, ownershipFailure, _ := toolhealth.Run(ctx, timeout, func(runCtx context.Context) (string, error) {
		return runner.Run(
			runCtx, path, "timeline", strings.TrimSpace(request.MemoryID),
			"--before", "0", "--after", "0",
			"--project", strings.TrimSpace(request.Project),
		)
	})
	if ownershipFailure.Kind != toolhealth.FailureNone {
		return memory.DeleteResult{}, fmt.Errorf(
			"verify Engram memory ownership [%s]: %s",
			ownershipFailure.Kind, ownershipFailure.Detail,
		)
	}

	args := []string{"delete", strings.TrimSpace(request.MemoryID)}
	if request.Hard {
		args = append(args, "--hard")
	}
	output, failure, _ := toolhealth.Run(ctx, timeout, func(runCtx context.Context) (string, error) {
		return runner.Run(runCtx, path, args...)
	})
	if failure.Kind != toolhealth.FailureNone {
		return memory.DeleteResult{}, fmt.Errorf("delete Engram memory [%s]: %s", failure.Kind, failure.Detail)
	}
	return memory.DeleteResult{
		Provider: ProviderID,
		Project: strings.TrimSpace(request.Project),
		MemoryID: strings.TrimSpace(request.MemoryID),
		Deleted: true,
		Hard: request.Hard,
		Summary: strings.TrimSpace(output),
	}, nil
}

func (c Client) Recall(ctx context.Context, request memory.RecallRequest) (memory.RecallResult, error) {
	if err := memory.ValidateRecallRequest(request); err != nil {
		return memory.RecallResult{}, err
	}
	path, runner, err := c.resolve()
	if err != nil {
		return memory.RecallResult{}, err
	}
	limit := request.Limit
	if limit == 0 {
		limit = 5
	}
	timeout := c.OperationTimeout
	if timeout <= 0 {
		timeout = toolhealth.DefaultOperationTimeout
	}
	output, failure, _ := toolhealth.Run(ctx, timeout, func(runCtx context.Context) (string, error) {
		return runner.Run(
			runCtx,
			path,
			"search",
			strings.TrimSpace(request.Query),
			"--project", strings.TrimSpace(request.Project),
			"--limit", fmt.Sprintf("%d", limit),
		)
	})
	if failure.Kind != toolhealth.FailureNone {
		return memory.RecallResult{}, fmt.Errorf("recall Engram memory [%s]: %s", failure.Kind, failure.Detail)
	}
	return memory.RecallResult{
		Provider: ProviderID,
		Project:  strings.TrimSpace(request.Project),
		Query:    strings.TrimSpace(request.Query),
		Context:  strings.TrimSpace(output),
	}, nil
}

func (c Client) resolve() (string, Runner, error) {
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
		return "", nil, fmt.Errorf("locate Engram binary: %w", err)
	}
	return path, runner, nil
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
