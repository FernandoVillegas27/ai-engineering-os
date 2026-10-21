package gitstage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/proposal"
	"github.com/FernandoVillegas27/ai-engineering-os/core/staging"
	"github.com/FernandoVillegas27/ai-engineering-os/integrations/commandmodel"
)

type Result struct {
	Workspace    string
	ChangedFiles []staging.PatchFile
}

type Stager struct {
	Runner commandmodel.Runner
}

func New() Stager {
	return Stager{Runner: commandmodel.OSRunner{}}
}

func (s Stager) Stage(ctx context.Context, projectRoot string, p proposal.Proposal) (Result, error) {
	files, err := staging.ValidateProposal(p, execution.DefaultSafeViewPolicy())
	if err != nil {
		return Result{}, err
	}

	root, err := filepath.Abs(projectRoot)
	if err != nil {
		return Result{}, fmt.Errorf("resolve project root: %w", err)
	}
	parent := filepath.Dir(root)
	workspace, err := os.MkdirTemp(parent, ".ai-engineering-os-stage-*")
	if err != nil {
		return Result{}, fmt.Errorf("create isolated stage workspace: %w", err)
	}

	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(workspace)
		}
	}()

	if _, err := execution.PrepareSafeReadOnlyView(root, workspace); err != nil {
		return Result{}, err
	}
	if err := verifyPreconditions(workspace, files); err != nil {
		return Result{}, err
	}

	runner := s.Runner
	if runner == nil {
		runner = commandmodel.OSRunner{}
	}
	gitPath, err := runner.LookPath("git")
	if err != nil {
		return Result{}, fmt.Errorf("git command not found in PATH: %w", err)
	}

	base := commandmodel.Invocation{
		Command: gitPath,
		Stdin:   p.Patch,
		Dir:     workspace,
		Env:     safeGitEnvironment(workspace),
	}
	check := base
	check.Args = []string{"apply", "--check", "--ignore-space-change", "--whitespace=nowarn", "-"}
	if _, stderr, err := runner.Run(ctx, check); err != nil {
		if strings.TrimSpace(stderr) != "" {
			return Result{}, fmt.Errorf("git apply --check failed: %w: %s", err, strings.TrimSpace(stderr))
		}
		return Result{}, fmt.Errorf("git apply --check failed: %w", err)
	}

	apply := base
	apply.Args = []string{"apply", "--ignore-space-change", "--whitespace=nowarn", "-"}
	if _, stderr, err := runner.Run(ctx, apply); err != nil {
		if strings.TrimSpace(stderr) != "" {
			return Result{}, fmt.Errorf("git apply failed: %w: %s", err, strings.TrimSpace(stderr))
		}
		return Result{}, fmt.Errorf("git apply failed: %w", err)
	}

	if err := verifyPostconditions(workspace, files); err != nil {
		return Result{}, err
	}

	cleanup = false
	return Result{Workspace: workspace, ChangedFiles: files}, nil
}

func verifyPreconditions(workspace string, files []staging.PatchFile) error {
	for _, file := range files {
		path := filepath.Join(workspace, filepath.FromSlash(file.Path))
		_, err := os.Stat(path)
		switch file.Action {
		case "create":
			if err == nil {
				return fmt.Errorf("create target already exists in stage workspace: %s", file.Path)
			}
			if !os.IsNotExist(err) {
				return err
			}
		case "modify", "delete":
			if err != nil {
				return fmt.Errorf("%s target unavailable in stage workspace %s: %w", file.Action, file.Path, err)
			}
		}
	}
	return nil
}

func verifyPostconditions(workspace string, files []staging.PatchFile) error {
	for _, file := range files {
		path := filepath.Join(workspace, filepath.FromSlash(file.Path))
		_, err := os.Stat(path)
		switch file.Action {
		case "create", "modify":
			if err != nil {
				return fmt.Errorf("%s target missing after apply %s: %w", file.Action, file.Path, err)
			}
		case "delete":
			if !os.IsNotExist(err) {
				return fmt.Errorf("delete target still exists after apply: %s", file.Path)
			}
		}
	}
	return nil
}

func safeGitEnvironment(workspace string) []string {
	allowed := []string{
		"SYSTEMROOT", "WINDIR", "COMSPEC", "TEMP", "TMP", "TMPDIR",
		"LANG", "LC_ALL", "TERM",
	}
	env := make([]string, 0, len(allowed)+2)
	for _, key := range allowed {
		if value, ok := os.LookupEnv(key); ok {
			env = append(env, key+"="+value)
		}
	}
	env = append(env,
		"HOME="+workspace,
		"XDG_CONFIG_HOME="+workspace,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_TERMINAL_PROMPT=0",
	)
	return env
}
