package commandmodel

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"

	"github.com/FernandoVillegas27/ai-engineering-os/core/modelprovider"
)

type Invocation struct {
	Command string
	Args    []string
	Stdin   string
	Dir     string
	Env     []string
}

type Adapter interface {
	ID() string
	Build(request modelprovider.ClassificationRequest, workDir string) (Invocation, error)
	Decode(workDir string) (modelprovider.ClassificationResponse, error)
}

type Runner interface {
	LookPath(file string) (string, error)
	Run(ctx context.Context, invocation Invocation) (stdout string, stderr string, err error)
}

type OSRunner struct{}

func (OSRunner) LookPath(file string) (string, error) {
	return exec.LookPath(file)
}

func (OSRunner) Run(ctx context.Context, invocation Invocation) (string, string, error) {
	command := exec.CommandContext(ctx, invocation.Command, invocation.Args...)
	command.Dir = invocation.Dir
	if invocation.Env != nil {
		command.Env = invocation.Env
	}
	command.Stdin = bytes.NewBufferString(invocation.Stdin)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr

	err := command.Run()
	return stdout.String(), stderr.String(), err
}

type Provider struct {
	Adapter Adapter
	Runner  Runner
	TempDir func(pattern string) (string, error)
	RemoveAll func(path string) error
}

func (p Provider) ID() string {
	if p.Adapter == nil {
		return "command"
	}
	return p.Adapter.ID()
}

func (p Provider) Classify(ctx context.Context, request modelprovider.ClassificationRequest) (modelprovider.ClassificationResponse, error) {
	if p.Adapter == nil {
		return modelprovider.ClassificationResponse{}, fmt.Errorf("command model adapter is required")
	}
	runner := p.Runner
	if runner == nil {
		runner = OSRunner{}
	}
	tempDir := p.TempDir
	if tempDir == nil {
		tempDir = defaultTempDir
	}
	removeAll := p.RemoveAll
	if removeAll == nil {
		removeAll = defaultRemoveAll
	}

	workDir, err := tempDir("ai-engineering-os-model-*")
	if err != nil {
		return modelprovider.ClassificationResponse{}, fmt.Errorf("create command provider workspace: %w", err)
	}
	defer removeAll(workDir)

	invocation, err := p.Adapter.Build(request, workDir)
	if err != nil {
		return modelprovider.ClassificationResponse{}, err
	}

	resolved, err := runner.LookPath(invocation.Command)
	if err != nil {
		return modelprovider.ClassificationResponse{}, fmt.Errorf("%s command not found in PATH: %w", invocation.Command, err)
	}
	invocation.Command = resolved

	_, stderr, runErr := runner.Run(ctx, invocation)
	if runErr != nil {
		if stderr != "" {
			return modelprovider.ClassificationResponse{}, fmt.Errorf("%s execution failed: %w: %s", p.Adapter.ID(), runErr, stderr)
		}
		return modelprovider.ClassificationResponse{}, fmt.Errorf("%s execution failed: %w", p.Adapter.ID(), runErr)
	}

	response, err := p.Adapter.Decode(workDir)
	if err != nil {
		return modelprovider.ClassificationResponse{}, fmt.Errorf("decode %s response: %w", p.Adapter.ID(), err)
	}
	return response, nil
}
