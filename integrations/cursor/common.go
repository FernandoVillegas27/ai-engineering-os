package cursor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime"
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
	return runAgentWithFormat(ctx, config, workspace, mode, prompt, "text")
}

func runAgentWithFormat(ctx context.Context, config RunnerConfig, workspace, mode, prompt, outputFormat string) (string, error) {
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

	outputFormat = strings.TrimSpace(outputFormat)
	if outputFormat == "" {
		outputFormat = "text"
	}
	args := []string{"-p", "--trust", "--output-format", outputFormat, "--sandbox", cursorSandboxMode(runtime.GOOS), "--workspace", workspace}
	if mode != "" {
		args = append(args, "--mode", mode)
	}
	if model := strings.TrimSpace(config.Model); model != "" {
		args = append(args, "--model", model)
	}
	args = append(args, prompt)

	plan := planAgentLaunch(runtime.GOOS, resolved)
	if plan.Kind == launchBatchUnresolved && promptNeedsSafeTransport(prompt) {
		return "", fmt.Errorf("cursor launcher %s is a Windows batch file and its node runtime could not be located; refusing to pass a multi-line prompt through cmd.exe because it would be truncated", resolved)
	}
	args = append(append([]string{}, plan.Args...), args...)

	timeout := config.Timeout
	if timeout <= 0 {
		timeout = defaultRuntimeTimeout
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	stdout, stderr, runErr := runner.Run(runCtx, commandmodel.Invocation{
		Command: plan.Command,
		Args:    args,
		Dir:     workspace,
		Env:     append(safeRuntimeEnvironment(), plan.Env...),
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


func decodeStreamFinalResponse(data string) (string, error) {
	lines := strings.Split(strings.ReplaceAll(data, "\r\n", "\n"), "\n")
	var assistant strings.Builder
	var terminalResult string
	terminalSeen := false
	terminalError := false

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var event struct {
			Type    string `json:"type"`
			Subtype string `json:"subtype"`
			IsError bool   `json:"is_error"`
			Result  string `json:"result"`
			Message struct {
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"message"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			return "", fmt.Errorf("parse Cursor stream-json event: %w", err)
		}

		switch event.Type {
		case "assistant":
			for _, block := range event.Message.Content {
				if block.Type == "text" {
					assistant.WriteString(block.Text)
				}
			}
		case "result":
			terminalSeen = true
			terminalError = event.IsError || (event.Subtype != "" && event.Subtype != "success")
			terminalResult = event.Result
		}
	}

	if !terminalSeen {
		return "", fmt.Errorf("Cursor stream-json ended without terminal result event")
	}
	if terminalError {
		return "", fmt.Errorf("Cursor stream-json reported an unsuccessful terminal result")
	}
	if strings.TrimSpace(terminalResult) != "" {
		return strings.TrimSpace(terminalResult), nil
	}
	if strings.TrimSpace(assistant.String()) != "" {
		return strings.TrimSpace(assistant.String()), nil
	}
	return "", fmt.Errorf("Cursor stream-json contained no final assistant text")
}

func decodeModelJSONObject(finalText string, target any) error {
	if err := decodeJSONObject(finalText, target); err != nil {
		return fmt.Errorf("%w; final response sample: %q", err, responseSample(finalText, 480))
	}
	return nil
}

func responseSample(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit] + "..."
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


func cursorSandboxMode(goos string) string {
	if strings.EqualFold(strings.TrimSpace(goos), "windows") {
		return "disabled"
	}
	return "enabled"
}
