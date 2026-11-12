package localverify

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/FernandoVillegas27/ai-engineering-os/core/verification"
)

type goTestEvent struct {
	Action      string `json:"Action"`
	Package     string `json:"Package"`
	Test        string `json:"Test"`
	Output      string `json:"Output"`
	ImportPath  string `json:"ImportPath"`
	FailedBuild string `json:"FailedBuild"`
}

type goTestParse struct {
	// Rendered is the human-readable text equivalent to plain go test output.
	Rendered   string
	Failures   []verification.Failure
	Confidence verification.ParseConfidence
}

type unitKey struct{ pkg, test string }

// parseGoTestJSON understands a go test -json stream. roots are workspace paths
// removed from signatures so identical failures match across different workspaces.
func parseGoTestJSON(raw string, exitFailed bool, roots ...string) goTestParse {
	return parseGoTestJSONWithStderr(raw, "", exitFailed, roots...)
}

// parseGoTestJSONWithStderr also accepts stderr. Go before 1.24 prints compile
// errors there instead of in the JSON stream, so build-failure signatures need it.
func parseGoTestJSONWithStderr(raw, stderr string, exitFailed bool, roots ...string) goTestParse {
	var (
		rendered     strings.Builder
		outputs      = map[unitKey][]string{}
		buildOutputs = map[string][]string{}
		running      = map[unitKey]bool{}
		started      = map[string]bool{}
		done         = map[string]bool{}
		failedTests  []unitKey
		seenTest     = map[unitKey]bool{}
		failedPkgs   []string
		pkgBuild     = map[string]string{}
		seenPkg      = map[string]bool{}
		buildFailed  []string
		valid        int
		invalid      int
	)

	for _, line := range strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var event goTestEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil || event.Action == "" {
			invalid++
			rendered.WriteString(line + "\n")
			continue
		}
		// Lifecycle events require a package, while build events require an
		// import path. Otherwise their failures can collapse into an empty
		// identity and must not authorize complete quality evidence.
		switch event.Action {
		case "output":
			// A nonempty but whitespace-only test name cannot identify a test.
			if event.Test != "" && strings.TrimSpace(event.Test) == "" {
				invalid++
				continue
			}
			if strings.TrimSpace(event.Package) == "" {
				invalid++
				continue
			}
		case "start", "run", "pass", "fail", "skip":
			// Go emits start only for packages. A test-scoped start is
			// malformed evidence and must not establish package lifecycle.
			// Likewise, run must identify a test or subtest.
			if (event.Action == "start" && event.Test != "") ||
				(event.Action == "run" && strings.TrimSpace(event.Test) == "") ||
				((event.Action == "pass" || event.Action == "fail" || event.Action == "skip") && event.Test != "" && strings.TrimSpace(event.Test) == "") {
				invalid++
				continue
			}
			if strings.TrimSpace(event.Package) == "" {
				invalid++
				continue
			}
			// FailedBuild names a build import path when present. A blank
			// base cannot identify the failed build reliably. FailedBuild is\n\t\t\t// package-scoped and must not be attached to a test failure.
			if event.Action == "fail" && event.FailedBuild != "" && (event.Test != "" || strings.TrimSpace(baseImportPath(event.FailedBuild)) == "") {
				invalid++
				continue
			}
		case "build-output", "build-fail":
			// Build events identify an import path, never an individual test.
			if event.Test != "" || strings.TrimSpace(event.ImportPath) == "" || strings.TrimSpace(baseImportPath(event.ImportPath)) == "" {
				invalid++
				continue
			}
		}
		valid++
		key := unitKey{event.Package, event.Test}

		switch event.Action {
		case "output":
			rendered.WriteString(event.Output)
			outputs[key] = append(outputs[key], event.Output)
		case "build-output":
			rendered.WriteString(event.Output)
			base := baseImportPath(event.ImportPath)
			buildOutputs[base] = append(buildOutputs[base], event.Output)
		case "build-fail":
			buildFailed = append(buildFailed, baseImportPath(event.ImportPath))
		case "start":
			started[event.Package] = true
		case "run":
			running[key] = true
		case "pass", "skip":
			if event.Test == "" {
				done[event.Package] = true
			} else {
				delete(running, key)
			}
		case "fail":
			if event.Test == "" {
				done[event.Package] = true
				if !seenPkg[event.Package] {
					seenPkg[event.Package] = true
					failedPkgs = append(failedPkgs, event.Package)
					pkgBuild[event.Package] = event.FailedBuild
				}
			} else {
				delete(running, key)
				if !seenTest[key] {
					seenTest[key] = true
					failedTests = append(failedTests, key)
				}
			}
		default:
			// Unknown event actions cannot be treated as fully parsed evidence.
			invalid++
		}
	}

	if valid == 0 {
		confidence := verification.ParseNotAttempted
		switch {
		case exitFailed:
			confidence = verification.ParseFailed
		case invalid > 0:
			// A malformed stream with a successful exit cannot establish
			// complete evidence or be mistaken for no attempted parse.
			confidence = verification.ParsePartial
		}
		return goTestParse{Rendered: raw, Confidence: confidence}
	}

	failures := map[string]verification.Failure{}
	add := func(failure verification.Failure) {
		if _, exists := failures[failure.ID]; !exists {
			failures[failure.ID] = failure
		}
	}

	testsPerPackage := map[string]int{}
	for _, key := range failedTests {
		testsPerPackage[key.pkg]++
		text := strings.Join(outputs[key], "")
		class := verification.FailureClassTest
		switch {
		case hasPanicLine(text):
			class = verification.FailureClassPanic
		case strings.Contains(key.test, "/"):
			class = verification.FailureClassSubtest
		}
		add(verification.Failure{
			ID:        key.pkg + "#" + key.test,
			Class:     class,
			Signature: verification.Signature(text, roots...),
		})
	}

	representedBuilds := map[string]bool{}
	stderrSections := legacyBuildSections(stderr)
	coarseBuild := false
	for _, pkg := range failedPkgs {
		if failedBuild := pkgBuild[pkg]; failedBuild != "" {
			base := baseImportPath(failedBuild)
			representedBuilds[base] = true
			text := strings.Join(buildOutputs[base], "")
			if text == "" {
				text = strings.Join(outputs[unitKey{pkg, ""}], "")
			}
			add(verification.Failure{ID: pkg, Class: verification.FailureClassBuild, Signature: verification.Signature(text, roots...)})
			continue
		}
		if packageOutput := outputs[unitKey{pkg, ""}]; isLegacyBuildFailure(packageOutput) {
			detail := stderr
			if section, found := stderrSections[pkg]; found {
				detail = section
			}
			if strings.TrimSpace(stderr) == "" {
				// Without the compiler text the signature cannot tell two build errors apart.
				coarseBuild = true
			}
			add(verification.Failure{
				ID:        pkg,
				Class:     verification.FailureClassBuild,
				Signature: verification.Signature(strings.Join(packageOutput, "")+"\n"+detail, roots...),
			})
			continue
		}
		// A package line that only repeats failing tests adds no information.
		if testsPerPackage[pkg] > 0 && isDerivedPackageFailure(pkg, outputs[unitKey{pkg, ""}]) {
			continue
		}
		add(verification.Failure{
			ID:        pkg,
			Class:     verification.FailureClassPackage,
			Signature: verification.Signature(strings.Join(outputs[unitKey{pkg, ""}], ""), roots...),
		})
	}
	for _, base := range buildFailed {
		if representedBuilds[base] {
			continue
		}
		representedBuilds[base] = true
		add(verification.Failure{ID: base, Class: verification.FailureClassBuild, Signature: verification.Signature(strings.Join(buildOutputs[base], ""), roots...)})
	}

	ids := make([]string, 0, len(failures))
	for id := range failures {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	ordered := make([]verification.Failure, 0, len(ids))
	for _, id := range ids {
		ordered = append(ordered, failures[id])
	}

	incomplete := len(running) > 0 || coarseBuild
	for pkg := range started {
		if !done[pkg] {
			incomplete = true
		}
	}

	confidence := verification.ParseComplete
	switch {
	case invalid > 0 || incomplete:
		confidence = verification.ParsePartial
	case exitFailed && len(ordered) == 0:
		confidence = verification.ParseFailed
	}
	return goTestParse{Rendered: rendered.String(), Failures: ordered, Confidence: confidence}
}

// baseImportPath turns "pkg [pkg.test]" into "pkg".
func baseImportPath(value string) string {
	if index := strings.Index(value, " ["); index >= 0 {
		return value[:index]
	}
	return value
}

// isLegacyBuildFailure recognizes the package summary go test prints when it could
// not build or set up a package, without including the compiler errors.
func isLegacyBuildFailure(outputs []string) bool {
	for _, line := range strings.Split(strings.Join(outputs, ""), "\n") {
		if strings.Contains(line, "[build failed]") || strings.Contains(line, "[setup failed]") {
			return true
		}
	}
	return false
}

// legacyBuildSections groups stderr by the "# import/path" headers the go command prints.
func legacyBuildSections(stderr string) map[string]string {
	sections := map[string]string{}
	var current string
	var text strings.Builder
	flush := func() {
		if current != "" {
			sections[current] += text.String()
		}
		text.Reset()
	}
	for _, line := range strings.Split(strings.ReplaceAll(stderr, "\r\n", "\n"), "\n") {
		if header, isHeader := strings.CutPrefix(line, "# "); isHeader {
			flush()
			current = baseImportPath(strings.TrimSpace(header))
			text.WriteString(line + "\n")
			continue
		}
		if current != "" {
			text.WriteString(line + "\n")
		}
	}
	flush()
	return sections
}

func hasPanicLine(text string) bool {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "panic: ") || strings.HasPrefix(line, "fatal error: ") {
			return true
		}
	}
	return false
}

// isDerivedPackageFailure reports whether the package output holds only the
// standard "FAIL" summary lines go test prints after failing tests.
func isDerivedPackageFailure(pkg string, outputs []string) bool {
	for _, line := range strings.Split(strings.Join(outputs, ""), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "", line == "FAIL":
		case strings.HasPrefix(line, "FAIL\t"+pkg+"\t"):
		default:
			return false
		}
	}
	return true
}
