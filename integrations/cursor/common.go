package cursor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/integrations/commandmodel"
)

const (
	RuntimeID             = "cursor-cli"
	defaultRuntimeTimeout = 5 * time.Minute
)

type RunnerConfig struct {
	Binary  string
	Model   string
	Runner  commandmodel.Runner
	Timeout time.Duration
}

func defaultConfig() RunnerConfig {
	return RunnerConfig{
		Binary:  "agent",
		Runner:  commandmodel.OSRunner{},
		Timeout: defaultRuntimeTimeout,
	}
}

func runAgent(ctx context.Context, config RunnerConfig, workspace, mode, prompt string) (string, error) {
	runner := config.Runner
	if runner == nil {
		runner = commandmodel.OSRunner{}
	}
	binary := strings.TrimSpace(config.Binary)
	if binary == "" {
		binary = "agent"
	}
	resolved, err := runner.LookPath(binary)
	if err != nil {
		return "", fmt.Errorf("cursor agent command not found in PATH: %w", err)
	}

	args := []string{"-p", "--output-format", "text", "--sandbox", "enabled", "--workspace", workspace}
	if mode != "" {
		args = append(args, "--mode", mode)
	}
	if model := strings.TrimSpace(config.Model); model != "" {
		args = append(args, "--model", model)
	}
	args = append(args, prompt)

	timeout := config.Timeout
	if timeout <= 0 {
		timeout = defaultRuntimeTimeout
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	stdout, stderr, runErr := runner.Run(runCtx, commandmodel.Invocation{
		Command: resolved,
		Args:    args,
		Dir:     workspace,
		Env:     safeRuntimeEnvironment(),
	})
	if runErr != nil {
		if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
			return "", fmt.Errorf("Cursor runtime timed out after %s", timeout)
		}
		if strings.TrimSpace(stderr) != "" {
			return "", fmt.Errorf("Cursor runtime failed: %w: %s", runErr, strings.TrimSpace(stderr))
		}
		return "", fmt.Errorf("Cursor runtime failed: %w", runErr)
	}
	return strings.TrimSpace(stdout), nil
}

func decodeJSONObject(data string, target any) error {
	data = strings.TrimSpace(data)
	data = strings.TrimPrefix(data, "```json")
	data = strings.TrimPrefix(data, "```")
	data = strings.TrimSuffix(data, "```")
	data = strings.TrimSpace(data)

	start := strings.Index(data, "{")
	end := strings.LastIndex(data, "}")
	if start < 0 || end < start {
		return fmt.Errorf("Cursor response did not contain a JSON object")
	}
	if err := json.Unmarshal([]byte(data[start:end+1]), target); err != nil {
		return fmt.Errorf("parse Cursor JSON response: %w", err)
	}
	return nil
}

func safeRuntimeEnvironment() []string {
	allowed := []string{
		"PATH", "PATHEXT", "SYSTEMROOT", "WINDIR", "COMSPEC",
		"TEMP", "TMP", "TMPDIR", "USERPROFILE", "HOME",
		"APPDATA", "LOCALAPPDATA", "PROGRAMDATA",
		"PROGRAMFILES", "PROGRAMFILES(X86)", "PROGRAMW6432",
		"LANG", "LC_ALL", "TERM",
	}
	env := make([]string, 0, len(allowed))
	for _, key := range allowed {
		if value, ok := os.LookupEnv(key); ok {
			env = append(env, key+"="+value)
		}
	}
	return env
}
