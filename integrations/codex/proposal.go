package codex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/proposal"
	"github.com/FernandoVillegas27/ai-engineering-os/integrations/commandmodel"
)

const (
	proposalSchemaFileName = "proposal.schema.json"
	proposalResultFileName = "proposal-result.json"
)

type ProposalRuntime struct {
	Binary  string
	Model   string
	Runner  commandmodel.Runner
	Timeout time.Duration
}

func NewProposalRuntime() ProposalRuntime {
	return ProposalRuntime{
		Binary:  "codex",
		Runner:  commandmodel.OSRunner{},
		Timeout: 2 * time.Minute,
	}
}

func (r ProposalRuntime) ID() string {
	return RuntimeID
}

func (r ProposalRuntime) Propose(ctx context.Context, unit execution.WorkUnit) (proposal.Proposal, error) {
	if err := execution.ValidateReadOnlyUnit(unit); err != nil {
		return proposal.Proposal{}, fmt.Errorf("invalid Codex proposal work unit: %w", err)
	}
	if strings.TrimSpace(unit.ProjectRoot) == "" {
		return proposal.Proposal{}, fmt.Errorf("project root is required")
	}

	runner := r.Runner
	if runner == nil {
		runner = commandmodel.OSRunner{}
	}

	projectRoot, err := filepath.Abs(unit.ProjectRoot)
	if err != nil {
		return proposal.Proposal{}, fmt.Errorf("resolve project root: %w", err)
	}
	tempParent := filepath.Dir(projectRoot)
	tempDir, err := os.MkdirTemp(tempParent, ".ai-engineering-os-proposal-*")
	if err != nil {
		return proposal.Proposal{}, fmt.Errorf("create Codex proposal workspace beside project: %w", err)
	}
	defer os.RemoveAll(tempDir)

	safeViewPath := filepath.Join(tempDir, "project")
	safeView, err := execution.PrepareSafeReadOnlyView(projectRoot, safeViewPath)
	if err != nil {
		return proposal.Proposal{}, err
	}

	schemaPath := filepath.Join(tempDir, proposalSchemaFileName)
	resultPath := filepath.Join(tempDir, proposalResultFileName)
	schema, err := proposalSchema()
	if err != nil {
		return proposal.Proposal{}, err
	}
	if err := os.WriteFile(schemaPath, schema, 0o600); err != nil {
		return proposal.Proposal{}, fmt.Errorf("write Codex proposal schema: %w", err)
	}

	binary := strings.TrimSpace(r.Binary)
	if binary == "" {
		binary = "codex"
	}
	resolved, err := runner.LookPath(binary)
	if err != nil {
		return proposal.Proposal{}, fmt.Errorf("codex command not found in PATH: %w", err)
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
		Stdin:   proposalPrompt(unit, safeView),
		Dir:     safeView.Root,
		Env:     safeRuntimeEnvironment(),
	}

	timeout := r.Timeout
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	_, stderr, runErr := runner.Run(runCtx, invocation)
	if runErr != nil {
		if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
			return proposal.Proposal{}, fmt.Errorf("Codex proposal timed out after %s", timeout)
		}
		if strings.TrimSpace(stderr) != "" {
			return proposal.Proposal{}, fmt.Errorf("Codex proposal failed: %w: %s", runErr, stderr)
		}
		return proposal.Proposal{}, fmt.Errorf("Codex proposal failed: %w", runErr)
	}

	data, err := os.ReadFile(resultPath)
	if err != nil {
		return proposal.Proposal{}, fmt.Errorf("read Codex proposal result: %w", err)
	}
	var result proposal.Proposal
	if err := json.Unmarshal(data, &result); err != nil {
		return proposal.Proposal{}, fmt.Errorf("parse Codex proposal result: %w", err)
	}
	return result, nil
}

func proposalPrompt(unit execution.WorkUnit, safeView execution.SafeViewReport) string {
	var b strings.Builder
	b.WriteString("Act as the assigned AI Engineering OS agent and PROPOSE a software change without applying it.\n")
	b.WriteString("You are in a READ-ONLY sanitized project view. Do not modify, create, delete, rename, format, patch, commit, push, install, or reconfigure files.\n")
	b.WriteString("Inspect the project and return a concrete implementation proposal as structured JSON only.\n")
	b.WriteString("The Patch field must contain a unified diff representing the proposed changes. It is proposal text only and must not be applied.\n")
	b.WriteString("Use repository-relative paths only. Do not include excluded secrets or attempt to reconstruct sanitized content.\n")
	b.WriteString("Describe Summary, file rationales, risks, and verification steps in Spanish. Keep code inside Patch in the project's natural language.\n")
	b.WriteString("If the requested change cannot be proposed safely from available evidence, explain that in Summary and return an empty Patch.\n\n")
	b.WriteString("Work unit:\n")
	b.WriteString("task: " + unit.Task + "\n")
	b.WriteString("intent: " + string(unit.Intent) + "\n")
	b.WriteString("agent: " + unit.AgentID + "\n")
	b.WriteString("capabilities: " + strings.Join(unit.Capabilities, ", ") + "\n")
	b.WriteString("enabled skills: " + strings.Join(unit.Skills, ", ") + "\n")
	b.WriteString(fmt.Sprintf("safe view copied files: %d\n", safeView.CopiedFiles))
	return b.String()
}

func proposalSchema() ([]byte, error) {
	schema := map[string]any{
		"type": "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"Summary": map[string]any{"type": "string"},
			"Files": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"additionalProperties": false,
					"properties": map[string]any{
						"Path": map[string]any{"type": "string"},
						"Action": map[string]any{"type": "string", "enum": []string{"create", "modify", "delete"}},
						"Rationale": map[string]any{"type": "string"},
					},
					"required": []string{"Path", "Action", "Rationale"},
				},
			},
			"Patch": map[string]any{"type": "string"},
			"Risks": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"additionalProperties": false,
					"properties": map[string]any{
						"Severity": map[string]any{"type": "string", "enum": []string{"info", "low", "medium", "high", "critical"}},
						"Detail": map[string]any{"type": "string"},
					},
					"required": []string{"Severity", "Detail"},
				},
			},
			"Verification": map[string]any{
				"type": "array",
				"items": map[string]any{"type": "string"},
			},
		},
		"required": []string{"Summary", "Files", "Patch", "Risks", "Verification"},
	}
	data, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal Codex proposal schema: %w", err)
	}
	return data, nil
}
