package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/integrations/commandmodel"
)

const (
	RuntimeID               = "codex-cli"
	executionSchemaFileName = "execution.schema.json"
	executionResultFileName = "execution-result.json"
)

type ExecutionRuntime struct {
	Binary string
	Model  string
	Runner commandmodel.Runner
}

func NewExecutionRuntime() ExecutionRuntime {
	return ExecutionRuntime{
		Binary: "codex",
		Runner: commandmodel.OSRunner{},
	}
}

func (r ExecutionRuntime) ID() string {
	return RuntimeID
}

func (r ExecutionRuntime) Analyze(ctx context.Context, unit execution.WorkUnit) (execution.Analysis, error) {
	if unit.Mode != execution.ModeReadOnly {
		return execution.Analysis{}, fmt.Errorf("Codex execution runtime requires read-only mode")
	}
	if strings.TrimSpace(unit.ProjectRoot) == "" {
		return execution.Analysis{}, fmt.Errorf("project root is required")
	}

	runner := r.Runner
	if runner == nil {
		runner = commandmodel.OSRunner{}
	}

	tempDir, err := os.MkdirTemp("", "ai-engineering-os-execution-*")
	if err != nil {
		return execution.Analysis{}, fmt.Errorf("create Codex execution workspace: %w", err)
	}
	defer os.RemoveAll(tempDir)

	schemaPath := filepath.Join(tempDir, executionSchemaFileName)
	resultPath := filepath.Join(tempDir, executionResultFileName)

	schema, err := executionSchema()
	if err != nil {
		return execution.Analysis{}, err
	}
	if err := os.WriteFile(schemaPath, schema, 0o600); err != nil {
		return execution.Analysis{}, fmt.Errorf("write Codex execution schema: %w", err)
	}

	binary := strings.TrimSpace(r.Binary)
	if binary == "" {
		binary = "codex"
	}
	resolved, err := runner.LookPath(binary)
	if err != nil {
		return execution.Analysis{}, fmt.Errorf("codex command not found in PATH: %w", err)
	}

	args := []string{
		"exec",
		"--ephemeral",
		"--skip-git-repo-check",
		"--sandbox", "read-only",
		"-c", `approval_policy="never"`,
		"--output-schema", schemaPath,
		"--output-last-message", resultPath,
	}
	if model := strings.TrimSpace(r.Model); model != "" {
		args = append(args, "--model", model)
	}
	args = append(args, "-")

	invocation := commandmodel.Invocation{
		Command: resolved,
		Args:    args,
		Stdin:   executionPrompt(unit),
		Dir:     unit.ProjectRoot,
	}

	_, stderr, runErr := runner.Run(ctx, invocation)
	if runErr != nil {
		if strings.TrimSpace(stderr) != "" {
			return execution.Analysis{}, fmt.Errorf("Codex execution failed: %w: %s", runErr, stderr)
		}
		return execution.Analysis{}, fmt.Errorf("Codex execution failed: %w", runErr)
	}

	data, err := os.ReadFile(resultPath)
	if err != nil {
		return execution.Analysis{}, fmt.Errorf("read Codex execution result: %w", err)
	}

	var result execution.Analysis
	if err := json.Unmarshal(data, &result); err != nil {
		return execution.Analysis{}, fmt.Errorf("parse Codex execution result: %w", err)
	}
	return result, nil
}

func executionPrompt(unit execution.WorkUnit) string {
	var b strings.Builder
	b.WriteString("Act as the assigned AI Engineering OS agent and perform a READ-ONLY analysis of the current project.\n")
	b.WriteString("You may inspect project files and run read-only inspection commands, but you must not modify, create, delete, rename, format, patch, commit, push, install, or reconfigure anything.\n")
	b.WriteString("Do not access secrets, credentials, environment values, or production systems.\n")
	b.WriteString("Return only the structured result required by the supplied JSON schema.\n")
	b.WriteString("Findings must be grounded in evidence from the project. If there are no findings, return an empty findings array.\n\n")

	b.WriteString("Work unit:\n")
	b.WriteString("task: ")
	b.WriteString(unit.Task)
	b.WriteString("\nintent: ")
	b.WriteString(string(unit.Intent))
	b.WriteString("\nagent: ")
	b.WriteString(unit.AgentID)
	b.WriteString("\ncapabilities: ")
	b.WriteString(strings.Join(unit.Capabilities, ", "))
	b.WriteString("\nenabled skills: ")
	b.WriteString(strings.Join(unit.Skills, ", "))
	b.WriteString("\nlogical tools: ")
	b.WriteString(strings.Join(unit.Tools, ", "))
	b.WriteString("\n")
	return b.String()
}

func executionSchema() ([]byte, error) {
	schema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"Summary": map[string]any{
				"type": "string",
			},
			"Findings": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"properties": map[string]any{
						"Severity": map[string]any{
							"type": "string",
							"enum": []string{"info", "low", "medium", "high", "critical"},
						},
						"Title": map[string]any{
							"type": "string",
						},
						"Detail": map[string]any{
							"type": "string",
						},
					},
					"required": []string{"Severity", "Title", "Detail"},
				},
			},
			"Evidence": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"properties": map[string]any{
						"Path": map[string]any{
							"type": "string",
						},
						"Detail": map[string]any{
							"type": "string",
						},
					},
					"required": []string{"Path", "Detail"},
				},
			},
		},
		"required": []string{"Summary", "Findings", "Evidence"},
	}

	data, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal Codex execution schema: %w", err)
	}
	return data, nil
}
