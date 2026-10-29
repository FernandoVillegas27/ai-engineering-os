package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	cursorintegration "github.com/FernandoVillegas27/ai-engineering-os/integrations/cursor"
)

type diagnoseOptions struct {
	root         string
	task         string
	binary       string
	responseFile string
	jsonOutput   bool
}

const diagnoseUsage = "usage: ai diagnose cursor-proposal [project-path] --task <task> [--binary agent] [--response-file <file>|-] [--json]"

func runDiagnose(args []string) error {
	if len(args) == 0 || args[0] != "cursor-proposal" {
		return fmt.Errorf(diagnoseUsage)
	}
	options, err := parseDiagnoseArgs(args[1:])
	if err != nil {
		return err
	}

	root, err := filepath.Abs(options.root)
	if err != nil {
		return fmt.Errorf("resolve project path: %w", err)
	}

	var response string
	switch options.responseFile {
	case "":
	case "-":
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("read response from stdin: %w", err)
		}
		response = string(data)
	default:
		data, err := os.ReadFile(options.responseFile)
		if err != nil {
			return fmt.Errorf("read response file: %w", err)
		}
		response = string(data)
	}

	config := cursorintegration.NewProposalRuntime().Config
	if options.binary != "" {
		config.Binary = options.binary
	}
	report, err := cursorintegration.DiagnoseProposal(context.Background(), cursorintegration.DiagnoseOptions{
		Config: config,
		Unit: execution.WorkUnit{
			ID:          "diagnose-cursor-proposal",
			Task:        options.task,
			ProjectRoot: root,
			AgentID:     "diagnose",
			Mode:        execution.ModeReadOnly,
			Permissions: execution.PermissionEnvelope{
				SourceCode: "read", Tests: "read", GitCommit: "deny",
				GitPush: "deny", Secrets: "deny", Production: "deny",
			},
		},
		ResponseText: response,
	})
	if err != nil {
		return err
	}

	if options.jsonOutput {
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
	} else {
		printDiagnoseReport(report)
	}
	if len(report.Problems) > 0 {
		return fmt.Errorf("diagnosis found %d problem(s)", len(report.Problems))
	}
	return nil
}

func printDiagnoseReport(report cursorintegration.DiagnoseReport) {
	fmt.Println("Cursor proposal diagnosis (no model invoked)")
	fmt.Printf("  binary: %s\n", report.Binary)
	fmt.Printf("  launch: %s (%s)\n", report.LaunchKind, report.LaunchCommand)
	fmt.Printf("  prompt: bytes=%d sha256=%s task_in_prompt=%t contract_in_prompt=%t\n", report.PromptBytes, report.PromptSHA256, report.TaskInPrompt, report.ContractInPrompt)
	fmt.Printf("  sanitized files: %d\n", report.SanitizedFiles)
	fmt.Printf("  transport: checked=%t intact=%t %s\n", report.TransportChecked, report.TransportIntact, report.TransportDetail)
	if report.ResponseChecked {
		fmt.Printf("  response: valid=%t %s\n", report.ResponseValid, report.ResponseDetail)
	}
	for _, problem := range report.Problems {
		fmt.Printf("  PROBLEM: %s\n", problem)
	}
	if len(report.Problems) == 0 {
		fmt.Println("  result: OK")
	}
}

func parseDiagnoseArgs(args []string) (diagnoseOptions, error) {
	options := diagnoseOptions{root: "."}
	var positional []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--task", "--binary", "--response-file":
			if i+1 >= len(args) {
				return diagnoseOptions{}, fmt.Errorf("%s requires a value", args[i])
			}
			i++
			switch args[i-1] {
			case "--task":
				options.task = args[i]
			case "--binary":
				options.binary = strings.TrimSpace(args[i])
			default:
				options.responseFile = args[i]
			}
		case "--json":
			options.jsonOutput = true
		case "-h", "--help":
			return diagnoseOptions{}, fmt.Errorf(diagnoseUsage)
		default:
			if strings.HasPrefix(args[i], "-") && args[i] != "-" {
				return diagnoseOptions{}, fmt.Errorf("unknown option: %s", args[i])
			}
			positional = append(positional, args[i])
		}
	}
	if len(positional) > 1 {
		return diagnoseOptions{}, fmt.Errorf("expected a single project path")
	}
	if len(positional) == 1 {
		options.root = positional[0]
	}
	if strings.TrimSpace(options.task) == "" {
		return diagnoseOptions{}, fmt.Errorf("--task is required")
	}
	return options, nil
}
