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

type parserKind int

const (
	parserNone parserKind = iota
	parserGoJSON
	parserJava
	parserNodeJSON
	parserPytestJUnit
)

const verifyReportToken = "{AI_VERIFY_REPORT}"

type plannedCheck struct {
	name       string
	command    string
	args       []string
	parser     parserKind
	reportFile string
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

	env, cleanupEnv, err := prepareVerificationEnvironment(root)
	if err != nil {
		return verification.Report{}, err
	}
	defer cleanupEnv()

	report := verification.Report{Status: verification.StatusPassed}
	cacheRoot := cacheRootFromEnv(env)
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

		args, reportPath := materializeCheckArgs(planned, cacheRoot)
		runCtx, cancel := context.WithTimeout(ctx, timeout)
		started := time.Now()
		stdout, stderr, runErr := runner.Run(runCtx, commandmodel.Invocation{
			Command: resolved,
			Args:    args,
			Dir:     root,
			Env:     env,
		})
		duration := time.Since(started)
		cancel()

		// Parse the complete stream before truncating anything for presentation.
		var failures []verification.Failure
		parseConfidence := verification.ParseNotAttempted
		switch planned.parser {
		case parserGoJSON:
			parsed := parseGoTestJSONWithStderr(stdout, stderr, runErr != nil, root, cacheRootFromEnv(env))
			stdout = parsed.Rendered
			failures = parsed.Failures
			parseConfidence = parsed.Confidence
		case parserJava:
			parsed := parseJavaTestOutput(stdout, stderr, runErr != nil, root, cacheRootFromEnv(env))
			failures = parsed.Failures
			parseConfidence = parsed.Confidence
		case parserNodeJSON:
			var parsed structuredParse
			if reportPath != "" {
				parsed = parseNodeTestJSONFile(reportPath, runErr != nil, root, cacheRoot)
			} else {
				parsed = parseNodeTestJSON(stdout, stderr, runErr != nil, root, cacheRoot)
			}
			failures = parsed.Failures
			parseConfidence = parsed.Confidence
		case parserPytestJUnit:
			parsed := parsePytestJUnitFile(reportPath, runErr != nil, root, cacheRoot)
			failures = parsed.Failures
			parseConfidence = parsed.Confidence
		}

		check := verification.Check{
			Name:     planned.name,
			Command:  append([]string{planned.command}, args...),
			Stdout:   truncate(stdout),
			Stderr:   truncate(stderr),
			Duration: duration,
			Passed:   runErr == nil,
		}
		if planned.parser != parserNone {
			check.Failures = failures
			check.FailureParse = parseConfidence
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
			args:    []string{"test", "-json", "./..."},
			parser:  parserGoJSON,
		})
	}

	if fileExists(filepath.Join(root, "pom.xml")) {
		checks = append(checks, plannedCheck{
			name:    "maven-test",
			command: "mvn",
			args:    []string{"-q", "test"},
			parser:  parserJava,
		})
	} else if fileExists(filepath.Join(root, "build.gradle")) || fileExists(filepath.Join(root, "build.gradle.kts")) {
		checks = append(checks, plannedCheck{
			name:    "gradle-test",
			command: "gradle",
			args:    []string{"test", "--no-daemon", "--console=plain"},
			parser:  parserJava,
		})
	}

	if fileExists(filepath.Join(root, "pyproject.toml")) ||
		fileExists(filepath.Join(root, "pytest.ini")) ||
		fileExists(filepath.Join(root, "conftest.py")) {
		checks = append(checks, plannedCheck{
			name:       "pytest",
			command:    "python",
			args:       []string{"-m", "pytest", "-r", "fE", "--junitxml=" + verifyReportToken},
			parser:     parserPytestJUnit,
			reportFile: "pytest-junit.xml",
		})
	}

	packagePath := filepath.Join(root, "package.json")
	if fileExists(packagePath) {
		nodeCheck, err := nodeTestCheck(root, packagePath)
		if err != nil {
			return nil, err
		}
		if nodeCheck != nil {
			checks = append(checks, *nodeCheck)
		}
	}

	return checks, nil
}

func nodeTestCheck(root, packagePath string) (*plannedCheck, error) {
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
	parser := parserNone
	reportFile := ""
	lower := strings.ToLower(script)
	switch {
	case strings.Contains(lower, "vitest"):
		args = append(args, "--", "--run", "--reporter=json", "--outputFile="+verifyReportToken)
		parser = parserNodeJSON
		reportFile = "node-test-report.json"
	case strings.Contains(lower, "jest"):
		args = append(args, "--", "--runInBand", "--json", "--outputFile="+verifyReportToken)
		parser = parserNodeJSON
		reportFile = "node-test-report.json"
	case strings.Contains(lower, "ng test"):
		args = append(args, "--", "--watch=false")
		unitTest, err := angularUsesUnitTestBuilder(root)
		if err != nil {
			return nil, err
		}
		if unitTest {
			args = append(args, "--reporters=json", "--output-file="+verifyReportToken)
			parser = parserNodeJSON
			reportFile = "angular-test-report.json"
		}
	}
	return &plannedCheck{
		name: "npm-test", command: "npm", args: args, parser: parser, reportFile: reportFile,
	}, nil
}

func angularUsesUnitTestBuilder(root string) (bool, error) {
	path := filepath.Join(root, "angular.json")
	if !fileExists(path) {
		return false, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("read angular.json: %w", err)
	}
	var workspace struct {
		Projects map[string]struct {
			Architect map[string]struct {
				Builder string `json:"builder"`
			} `json:"architect"`
			Targets map[string]struct {
				Builder string `json:"builder"`
			} `json:"targets"`
		} `json:"projects"`
	}
	if err := json.Unmarshal(data, &workspace); err != nil {
		return false, fmt.Errorf("parse angular.json: %w", err)
	}
	for _, project := range workspace.Projects {
		for _, targets := range []map[string]struct{ Builder string `json:"builder"` }{project.Architect, project.Targets} {
			if test, ok := targets["test"]; ok &&
				strings.EqualFold(strings.TrimSpace(test.Builder), "@angular/build:unit-test") {
				return true, nil
			}
		}
	}
	return false, nil
}

func materializeCheckArgs(planned plannedCheck, cacheRoot string) ([]string, string) {
	reportPath := ""
	if strings.TrimSpace(planned.reportFile) != "" {
		reportPath = filepath.Join(cacheRoot, planned.reportFile)
	}
	args := append([]string(nil), planned.args...)
	for i := range args {
		if strings.Contains(args[i], verifyReportToken) {
			args[i] = strings.ReplaceAll(args[i], verifyReportToken, reportPath)
		}
	}
	return args, reportPath
}

func safeVerificationEnvironment() []string {
	allowed := []string{
		"PATH", "PATHEXT", "SYSTEMROOT", "WINDIR", "COMSPEC",
		"TEMP", "TMP", "TMPDIR",
		"JAVA_HOME", "GOROOT", "GOMODCACHE",
		"LANG", "LC_ALL", "TERM",
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

func prepareVerificationEnvironment(root string) ([]string, func(), error) {
	parent := filepath.Dir(root)
	cacheRoot, err := os.MkdirTemp(parent, ".ai-engineering-os-verify-cache-*")
	if err != nil {
		return nil, func() {}, fmt.Errorf("create verification cache: %w", err)
	}

	cleanup := func() {
		_ = os.RemoveAll(cacheRoot)
	}

	goCache := filepath.Join(cacheRoot, "go-build")
	home := filepath.Join(cacheRoot, "home")
	goPath := filepath.Join(cacheRoot, "go-path")
	npmCache := filepath.Join(cacheRoot, "npm-cache")
	gradleHome := filepath.Join(cacheRoot, "gradle-home")
	mavenRepo := filepath.Join(cacheRoot, "maven-repository")
	for _, dir := range []string{goCache, home, goPath, npmCache, gradleHome, mavenRepo} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			cleanup()
			return nil, func() {}, fmt.Errorf("create verification isolation directory: %w", err)
		}
	}

	// Project-owned verification commands are explicitly trusted by the caller,
	// but they still receive an isolated user/cache home so normal tool lookup
	// cannot discover developer dotfiles or credential configuration through
	// HOME/USERPROFILE/GOPATH/npm/Gradle/Maven defaults.
	env := safeVerificationEnvironment()
	env = append(
		env,
		"GOCACHE="+goCache,
		"GOPATH="+goPath,
		"HOME="+home,
		"USERPROFILE="+home,
		"NPM_CONFIG_CACHE="+npmCache,
		"GRADLE_USER_HOME="+gradleHome,
		"MAVEN_OPTS=-Duser.home=\""+home+"\" -Dmaven.repo.local=\""+mavenRepo+"\"",
	)
	return env, cleanup, nil
}

func cacheRootFromEnv(env []string) string {
	for _, entry := range env {
		if value, ok := strings.CutPrefix(entry, "GOCACHE="); ok {
			return filepath.Dir(value)
		}
	}
	return ""
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
	text := strings.ToLower(stdout + "\n" + stderr)
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
		"build cache is required",
		"gocache is not defined",
	}
	for _, marker := range infrastructureMarkers {
		if strings.Contains(text, marker) {
			return verification.FailureInfrastructure, false
		}
	}
	return verification.FailureCode, true
}
