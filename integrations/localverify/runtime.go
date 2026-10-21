package localverify

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/verification"
	"github.com/FernandoVillegas27/ai-engineering-os/integrations/commandmodel"
)

const RuntimeID = "local-verifier"

type Runtime struct {
	Runner  commandmodel.Runner
	Timeout time.Duration
}

type plannedCheck struct {
	name    string
	command string
	args    []string
}

func New() Runtime {
	return Runtime{
		Runner:  commandmodel.OSRunner{},
		Timeout: 3 * time.Minute,
	}
}

func (Runtime) ID() string { return RuntimeID }

func (r Runtime) Verify(ctx context.Context, workspace string) (verification.Report, error) {
	root, err := filepath.Abs(workspace)
	if err != nil {
		return verification.Report{}, fmt.Errorf("resolve verification workspace: %w", err)
	}
	checks, err := planChecks(root)
	if err != nil {
		return verification.Report{}, err
	}
	if len(checks) == 0 {
		return verification.Report{
			Status: verification.StatusSkipped,
			Checks: []verification.Check{{
				Name:   "project-verification",
				Stdout: "No supported deterministic verification command was detected.",
				Passed: true,
			}},
		}, nil
	}

	runner := r.Runner
	if runner == nil {
		runner = commandmodel.OSRunner{}
	}
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = 3 * time.Minute
	}

	report := verification.Report{Status: verification.StatusPassed}
	for _, planned := range checks {
		resolved, err := runner.LookPath(planned.command)
		if err != nil {
			report.Status = verification.StatusFailed
			report.Checks = append(report.Checks, verification.Check{
				Name:    planned.name,
				Command: append([]string{planned.command}, planned.args...),
				ExitCode:    127,
				Stderr:      fmt.Sprintf("%s command not found in PATH: %v", planned.command, err),
				Passed:      false,
				FailureKind: verification.FailureInfrastructure,
				Correctable: false,
			})
			continue
		}

		runCtx, cancel := context.WithTimeout(ctx, timeout)
		started := time.Now()
		stdout, stderr, runErr := runner.Run(runCtx, commandmodel.Invocation{
			Command: resolved,
			Args:    planned.args,
			Dir:     root,
			Env:     safeVerificationEnvironment(),
		})
		duration := time.Since(started)
		cancel()

		check := verification.Check{
			Name:     planned.name,
			Command:  append([]string{planned.command}, planned.args...),
			Stdout:   truncate(stdout),
			Stderr:   truncate(stderr),
			Duration: duration,
			Passed:   runErr == nil,
		}
		if runErr != nil {
			check.ExitCode = 1
			if runCtx.Err() == context.DeadlineExceeded {
				check.Stderr = strings.TrimSpace(check.Stderr + "\nverification timed out after " + timeout.String())
				check.FailureKind = verification.FailureInfrastructure
				check.Correctable = false
			} else {
				check.FailureKind, check.Correctable = classifyFailure(check.Stdout, check.Stderr)
			}
			report.Status = verification.StatusFailed
		}
		report.Checks = append(report.Checks, check)
	}

	return report, nil
}

func planChecks(root string) ([]plannedCheck, error) {
	var checks []plannedCheck

	if fileExists(filepath.Join(root, "go.mod")) {
		checks = append(checks, plannedCheck{
			name:    "go-test",
			command: "go",
			args:    []string{"test", "./..."},
		})
	}

	if fileExists(filepath.Join(root, "pom.xml")) {
		checks = append(checks, plannedCheck{
			name:    "maven-test",
			command: "mvn",
			args:    []string{"-q", "test"},
		})
	} else if fileExists(filepath.Join(root, "build.gradle")) || fileExists(filepath.Join(root, "build.gradle.kts")) {
		checks = append(checks, plannedCheck{
			name:    "gradle-test",
			command: "gradle",
			args:    []string{"test", "--no-daemon"},
		})
	}

	if fileExists(filepath.Join(root, "pyproject.toml")) {
		checks = append(checks, plannedCheck{
			name:    "pytest",
			command: "python",
			args:    []string{"-m", "pytest"},
		})
	}

	packagePath := filepath.Join(root, "package.json")
	if fileExists(packagePath) {
		nodeCheck, err := nodeTestCheck(packagePath)
		if err != nil {
			return nil, err
		}
		if nodeCheck != nil {
			checks = append(checks, *nodeCheck)
		}
	}

	return checks, nil
}

func nodeTestCheck(packagePath string) (*plannedCheck, error) {
	data, err := os.ReadFile(packagePath)
	if err != nil {
		return nil, fmt.Errorf("read package.json: %w", err)
	}
	var manifest struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("parse package.json: %w", err)
	}
	script := strings.TrimSpace(manifest.Scripts["test"])
	if script == "" || strings.Contains(strings.ToLower(script), "no test specified") {
		return nil, nil
	}

	args := []string{"test"}
	lower := strings.ToLower(script)
	switch {
	case strings.Contains(lower, "vitest"):
		args = append(args, "--", "--run")
	case strings.Contains(lower, "jest"):
		args = append(args, "--", "--runInBand")
	case strings.Contains(lower, "ng test"):
		args = append(args, "--", "--watch=false")
	}
	return &plannedCheck{name: "npm-test", command: "npm", args: args}, nil
}

func safeVerificationEnvironment() []string {
	allowed := []string{
		"PATH", "PATHEXT", "SYSTEMROOT", "WINDIR", "COMSPEC",
		"TEMP", "TMP", "TMPDIR", "HOME", "USERPROFILE",
		"JAVA_HOME", "GOROOT", "GOPATH", "GOCACHE", "GOMODCACHE",
		"NPM_CONFIG_CACHE", "LANG", "LC_ALL", "TERM",
	}
	env := make([]string, 0, len(allowed)+1)
	for _, key := range allowed {
		if value, ok := os.LookupEnv(key); ok {
			env = append(env, key+"="+value)
		}
	}
	env = append(env, "CI=true")
	return env
}

func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	return err == nil && info.Mode().IsRegular()
}

func truncate(value string) string {
	const max = 12000
	if len(value) <= max {
		return value
	}
	return value[:max] + "\n...[truncated]"
}


func classifyFailure(stdout, stderr string) (verification.FailureKind, bool) {
	text := strings.ToLower(stdout + "
" + stderr)
	infrastructureMarkers := []string{
		"command not found",
		"not recognized as an internal or external command",
		"could not resolve host",
		"no such host",
		"network is unreachable",
		"temporary failure in name resolution",
		"proxyconnect tcp",
		"proxy authentication required",
		"407 proxy authentication",
		"connection timed out",
		"connection reset by peer",
		"failed to download",
		"unable to access",
	}
	for _, marker := range infrastructureMarkers {
		if strings.Contains(text, marker) {
			return verification.FailureInfrastructure, false
		}
	}
	return verification.FailureCode, true
}
