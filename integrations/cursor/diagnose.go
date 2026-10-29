package cursor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/proposal"
	"github.com/FernandoVillegas27/ai-engineering-os/integrations/commandmodel"
)

// argvProbeScript prints the byte length and SHA-256 of the last argument; it never contacts a model.
const argvProbeScript = `const c=require('crypto');const a=process.argv.slice(1);const p=a[a.length-1]||'';console.log(JSON.stringify({count:a.length,bytes:Buffer.byteLength(p),sha256:c.createHash('sha256').update(p).digest('hex')}))`

type DiagnoseOptions struct {
	Config RunnerConfig
	Unit   execution.WorkUnit
	// ResponseText is an optional captured Cursor response (stream-json or final text) to validate.
	ResponseText string
}

type DiagnoseReport struct {
	Binary           string   `json:"binary"`
	LaunchKind       string   `json:"launch_kind"`
	LaunchCommand    string   `json:"launch_command"`
	PromptBytes      int      `json:"prompt_bytes"`
	PromptSHA256     string   `json:"prompt_sha256"`
	TaskInPrompt     bool     `json:"task_in_prompt"`
	ContractInPrompt bool     `json:"contract_in_prompt"`
	SanitizedFiles   int      `json:"sanitized_files"`
	TransportChecked bool     `json:"transport_checked"`
	TransportIntact  bool     `json:"transport_intact"`
	TransportDetail  string   `json:"transport_detail"`
	ResponseChecked  bool     `json:"response_checked"`
	ResponseValid    bool     `json:"response_valid"`
	ResponseDetail   string   `json:"response_detail,omitempty"`
	Problems         []string `json:"problems"`
}

// DiagnoseProposal checks prompt integrity and the JSON contract without invoking any model.
func DiagnoseProposal(ctx context.Context, options DiagnoseOptions) (DiagnoseReport, error) {
	unit := options.Unit
	if err := execution.ValidateReadOnlyUnit(unit); err != nil {
		return DiagnoseReport{}, fmt.Errorf("invalid diagnose work unit: %w", err)
	}
	projectRoot, err := filepath.Abs(unit.ProjectRoot)
	if err != nil {
		return DiagnoseReport{}, fmt.Errorf("resolve project root: %w", err)
	}
	tempDir, err := os.MkdirTemp("", ".ai-engineering-os-cursor-diagnose-*")
	if err != nil {
		return DiagnoseReport{}, fmt.Errorf("create diagnose workspace: %w", err)
	}
	defer os.RemoveAll(tempDir)

	view, err := execution.PrepareSafeReadOnlyView(projectRoot, filepath.Join(tempDir, "project"))
	if err != nil {
		return DiagnoseReport{}, err
	}

	prompt := proposalPrompt(unit, view)
	sum := sha256.Sum256([]byte(prompt))
	report := DiagnoseReport{
		PromptBytes:      len(prompt),
		PromptSHA256:     hex.EncodeToString(sum[:]),
		TaskInPrompt:     strings.Contains(prompt, unit.Task),
		ContractInPrompt: strings.Contains(prompt, "Return EXACTLY one JSON object"),
		SanitizedFiles:   view.CopiedFiles,
		Problems:         []string{},
	}
	if !report.TaskInPrompt {
		report.Problems = append(report.Problems, "task text is missing from the generated prompt")
	}
	if !report.ContractInPrompt {
		report.Problems = append(report.Problems, "JSON contract is missing from the generated prompt")
	}

	config := options.Config
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
		report.Problems = append(report.Problems, fmt.Sprintf("cursor agent command not found in PATH: %v", err))
	} else {
		report.Binary = resolved
		diagnoseTransport(ctx, runner, resolved, prompt, sum[:], &report)
	}

	if strings.TrimSpace(options.ResponseText) != "" {
		diagnoseResponse(unit.Task, options.ResponseText, &report)
	}
	return report, nil
}

func diagnoseTransport(ctx context.Context, runner commandmodel.Runner, resolved, prompt string, wantSum []byte, report *DiagnoseReport) {
	plan := planAgentLaunch(runtime.GOOS, resolved)
	report.LaunchKind = plan.Kind
	report.LaunchCommand = plan.Command

	switch plan.Kind {
	case launchBatchUnresolved:
		report.Problems = append(report.Problems, "Windows batch launcher found but its node runtime could not be located; multi-line prompts would be truncated by cmd.exe")
		report.TransportDetail = "not probed"
	case launchDirect:
		report.TransportDetail = "native launcher passes argv verbatim; not probed"
	case launchWindowsNode:
		report.TransportChecked = true
		stdout, stderr, err := runner.Run(ctx, commandmodel.Invocation{
			Command: plan.Node,
			Args:    []string{"-e", argvProbeScript, prompt},
			Env:     append(safeRuntimeEnvironment(), plan.Env...),
		})
		if err != nil {
			report.TransportDetail = fmt.Sprintf("probe failed: %v %s", err, strings.TrimSpace(stderr))
			report.Problems = append(report.Problems, "argv transport probe could not run")
			return
		}
		var got struct {
			Count  int    `json:"count"`
			Bytes  int    `json:"bytes"`
			SHA256 string `json:"sha256"`
		}
		if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &got); err != nil {
			report.TransportDetail = "probe returned unparseable output"
			report.Problems = append(report.Problems, "argv transport probe returned unparseable output")
			return
		}
		report.TransportIntact = got.SHA256 == hex.EncodeToString(wantSum) && got.Bytes == len(prompt)
		report.TransportDetail = fmt.Sprintf("received bytes=%d sha256=%s", got.Bytes, got.SHA256)
		if !report.TransportIntact {
			report.Problems = append(report.Problems, "prompt received by the runtime differs from the prompt sent")
		}
	}
}

func diagnoseResponse(task, raw string, report *DiagnoseReport) {
	report.ResponseChecked = true
	finalText, err := decodeStreamFinalResponse(raw)
	if err != nil {
		finalText = raw
	}
	var result proposal.Proposal
	if err := decodeModelJSONObject(finalText, &result); err != nil {
		report.ResponseDetail = err.Error()
		report.Problems = append(report.Problems, "response does not satisfy the JSON contract")
		return
	}
	switch {
	case strings.TrimSpace(result.Summary) == "":
		report.ResponseDetail = "empty summary"
	case strings.TrimSpace(result.Patch) != "":
		report.ResponseDetail = "patch must be empty"
	default:
		if err := proposal.ValidateTaskAlignment(task, result); err != nil {
			report.ResponseDetail = "task alignment: " + err.Error()
		}
	}
	if report.ResponseDetail != "" {
		report.Problems = append(report.Problems, "response violates the proposal contract")
		return
	}
	report.ResponseValid = true
	report.ResponseDetail = "valid proposal contract"
}
