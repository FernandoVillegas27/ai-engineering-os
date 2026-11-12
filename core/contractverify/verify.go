package contractverify

import (
	"encoding/json"
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type FindingKind string

const (
	FindingOpenAPI      FindingKind = "openapi"
	FindingArchitecture FindingKind = "architecture"
)

type Finding struct {
	Kind    FindingKind `json:"kind"`
	ID      string      `json:"id"`
	Path    string      `json:"path"`
	Summary string      `json:"summary"`
}

type Result struct {
	Passed   bool      `json:"passed"`
	Findings []Finding `json:"findings,omitempty"`
}

func Verify(originalRoot, stagedRoot string) (Result, error) {
	originalRoot = strings.TrimSpace(originalRoot)
	stagedRoot = strings.TrimSpace(stagedRoot)
	if originalRoot == "" || stagedRoot == "" {
		return Result{}, errors.New("contract verification requires original and staged roots")
	}
	originalAbs, err := filepath.Abs(originalRoot)
	if err != nil {
		return Result{}, err
	}
	stagedAbs, err := filepath.Abs(stagedRoot)
	if err != nil {
		return Result{}, err
	}
	for _, root := range []string{originalAbs, stagedAbs} {
		if err := verifyRealDirectoryChain(root); err != nil {
			return Result{}, fmt.Errorf("inspect contract verification root: %w", err)
		}
	}

	var findings []Finding
	openAPI, err := verifyOpenAPIContracts(originalAbs, stagedAbs)
	if err != nil {
		return Result{}, err
	}
	findings = append(findings, openAPI...)

	architecture, err := verifyArchitecture(originalAbs, stagedAbs)
	if err != nil {
		return Result{}, err
	}
	findings = append(findings, architecture...)

	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Kind != findings[j].Kind {
			return findings[i].Kind < findings[j].Kind
		}
		if findings[i].Path != findings[j].Path {
			return findings[i].Path < findings[j].Path
		}
		return findings[i].ID < findings[j].ID
	})
	return Result{Passed: len(findings) == 0, Findings: findings}, nil
}

// verifyRealDirectoryChain rejects symlinks in any component of an absolute
// project root. This prevents an ancestor link from redirecting a workspace
// outside the intended tree. It is a pre-read guard, not an atomic open.
func verifyRealDirectoryChain(root string) error {
	current := filepath.Clean(root)
	// Check the root and its direct parent. macOS has legitimate system
	// ancestor symlinks (for example /var -> /private/var).
	for depth := 0; depth < 2; depth++ {
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("contract verification root path must not contain symbolic links: %s", current)
		}
		if !info.IsDir() {
			return fmt.Errorf("contract verification root path must contain a real directory at each component: %s", current)
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return nil
}

func verifyOpenAPIContracts(originalRoot, stagedRoot string) ([]Finding, error) {
	originalFiles, err := discoverOpenAPIFiles(originalRoot)
	if err != nil {
		return nil, err
	}
	var findings []Finding
	for _, relative := range originalFiles {
		if err := verifyContractFilePath(originalRoot, relative); err != nil {
			return nil, fmt.Errorf("inspect original OpenAPI %s: %w", relative, err)
		}
		before, err := loadOpenAPIDocument(filepath.Join(originalRoot, relative))
		if err != nil {
			return nil, fmt.Errorf("load original OpenAPI %s: %w", relative, err)
		}
		stagedPath := filepath.Join(stagedRoot, relative)
		if err := verifyContractFilePath(stagedRoot, relative); os.IsNotExist(err) {
			findings = append(findings, Finding{
				Kind: FindingOpenAPI, ID: "contract-removed", Path: relative,
				Summary: "existing OpenAPI contract file was removed",
			})
			continue
		} else if err != nil {
			return nil, fmt.Errorf("inspect staged OpenAPI %s: %w", relative, err)
		}
		after, err := loadOpenAPIDocument(stagedPath)
		if err != nil {
			return nil, fmt.Errorf("load staged OpenAPI %s: %w", relative, err)
		}
		findings = append(findings, compareOpenAPI(relative, before, after)...)
	}
	return findings, nil
}

func discoverOpenAPIFiles(root string) ([]string, error) {
	var values []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			// WalkDir skips linked directories; fail closed instead of silently
			// omitting a tree of contracts from compatibility verification.
			info, statErr := os.Stat(path)
			if statErr != nil {
				return fmt.Errorf("inspect OpenAPI symlink %s: %w", path, statErr)
			}
			if info.IsDir() {
				return fmt.Errorf("OpenAPI directory must not be a symbolic link: %s", path)
			}
		}
		if entry.IsDir() {
			name := entry.Name()
			if name == ".git" || name == "node_modules" || name == "vendor" || name == "build" || name == "dist" {
				return filepath.SkipDir
			}
			return nil
		}
		name := strings.ToLower(entry.Name())
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".json" && ext != ".yaml" && ext != ".yml" {
			return nil
		}
		if !strings.Contains(name, "openapi") && !strings.Contains(name, "swagger") {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		values = append(values, filepath.ToSlash(relative))
		return nil
	})
	sort.Strings(values)
	return values, err
}


// verifyContractFilePath refuses symlinked intermediate directories and
// final files. Contract verification must never follow links outside the
// original or staged project. The final component must be a regular file.
//
// This is a pre-read guard, not a substitute for platform-specific atomic
// no-follow file opening when defending against an actively mutating workspace.
func verifyContractFilePath(root, relative string) error {
	clean := filepath.Clean(filepath.FromSlash(relative))
	if filepath.IsAbs(clean) || clean == "." || clean == ".." ||
		strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return errors.New("contract path must remain repository-relative")
	}
	parts := strings.Split(clean, string(filepath.Separator))
	current := root
	for i, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("contract path contains symbolic link: %s", relative)
		}
		if i < len(parts)-1 && !info.IsDir() {
			return fmt.Errorf("contract parent must be a directory: %s", relative)
		}
		if i == len(parts)-1 && !info.Mode().IsRegular() {
			return fmt.Errorf("contract must be a regular file: %s", relative)
		}
	}
	return nil
}

// readStableContractFile checks the opened file identity against the validated
// path before and after reading. This detects ordinary path replacement during
// verification, but is not an atomic no-follow open and cannot defeat all races.
func readStableContractFile(path string) ([]byte, error) {
	before, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() {
		return nil, fmt.Errorf("contract file must be regular before read: %s", path)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !opened.Mode().IsRegular() || !os.SameFile(before, opened) {
		return nil, fmt.Errorf("contract file changed before read: %s", path)
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}
	after, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !after.Mode().IsRegular() || !os.SameFile(opened, after) || opened.Size() != int64(len(data)) {
		return nil, fmt.Errorf("contract file changed during read: %s", path)
	}
	return data, nil
}

func loadOpenAPIDocument(path string) (map[string]any, error) {
	data, err := readStableContractFile(path)
	if err != nil {
		return nil, err
	}
	var value any
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		err = json.Unmarshal(data, &value)
	default:
		err = yaml.Unmarshal(data, &value)
	}
	if err != nil {
		return nil, err
	}
	root, ok := stringMap(value)
	if !ok {
		return nil, errors.New("OpenAPI root must be an object")
	}
	if strings.TrimSpace(stringValue(root["openapi"])) == "" && strings.TrimSpace(stringValue(root["swagger"])) == "" {
		return nil, errors.New("document does not declare openapi/swagger version")
	}
	return root, nil
}

func compareOpenAPI(path string, before, after map[string]any) []Finding {
	beforePaths, _ := stringMap(before["paths"])
	afterPaths, _ := stringMap(after["paths"])
	var findings []Finding
	httpMethods := []string{"get", "post", "put", "patch", "delete", "head", "options", "trace"}

	for route, beforeRouteRaw := range beforePaths {
		beforeRoute, _ := stringMap(beforeRouteRaw)
		afterRoute, exists := stringMap(afterPaths[route])
		if !exists {
			findings = append(findings, Finding{
				Kind: FindingOpenAPI, ID: "path-removed:" + route, Path: path,
				Summary: "existing API path removed: " + route,
			})
			continue
		}
		for _, method := range httpMethods {
			beforeOperation, hasBefore := stringMap(beforeRoute[method])
			if !hasBefore {
				continue
			}
			afterOperation, hasAfter := stringMap(afterRoute[method])
			if !hasAfter {
				findings = append(findings, Finding{
					Kind: FindingOpenAPI, ID: "operation-removed:" + method + ":" + route, Path: path,
					Summary: strings.ToUpper(method) + " " + route + " was removed",
				})
				continue
			}
			findings = append(findings, compareOperation(path, method, route, beforeOperation, afterOperation)...)
		}
	}
	return findings
}

func compareOperation(path, method, route string, before, after map[string]any) []Finding {
	prefix := strings.ToUpper(method) + " " + route
	var findings []Finding

	beforeID := strings.TrimSpace(stringValue(before["operationId"]))
	afterID := strings.TrimSpace(stringValue(after["operationId"]))
	if beforeID != "" && afterID != beforeID {
		findings = append(findings, Finding{
			Kind: FindingOpenAPI, ID: "operation-id-changed:" + method + ":" + route, Path: path,
			Summary: prefix + " changed operationId",
		})
	}

	beforeResponses, _ := stringMap(before["responses"])
	afterResponses, _ := stringMap(after["responses"])
	for status, response := range beforeResponses {
		afterResponse, ok := afterResponses[status]
		if !ok {
			findings = append(findings, Finding{
				Kind: FindingOpenAPI, ID: "response-removed:" + method + ":" + route + ":" + status, Path: path,
				Summary: prefix + " removed response " + status,
			})
			continue
		}
		if canonicalJSON(response) != canonicalJSON(afterResponse) {
			findings = append(findings, Finding{
				Kind: FindingOpenAPI, ID: "response-changed:" + method + ":" + route + ":" + status, Path: path,
				Summary: prefix + " changed response " + status + " contract",
			})
		}
	}

	beforeParams := parameterIndex(before["parameters"])
	afterParams := parameterIndex(after["parameters"])
	for key, afterParam := range afterParams {
		beforeParam, existed := beforeParams[key]
		if !existed {
			if boolValue(afterParam["required"]) {
				findings = append(findings, Finding{
					Kind: FindingOpenAPI, ID: "required-parameter-added:" + method + ":" + route + ":" + key, Path: path,
					Summary: prefix + " added required parameter " + key,
				})
			}
			continue
		}
		if canonicalJSON(beforeParam["schema"]) != canonicalJSON(afterParam["schema"]) {
			findings = append(findings, Finding{
				Kind: FindingOpenAPI, ID: "parameter-schema-changed:" + method + ":" + route + ":" + key, Path: path,
				Summary: prefix + " changed parameter schema " + key,
			})
		}
		if !boolValue(beforeParam["required"]) && boolValue(afterParam["required"]) {
			findings = append(findings, Finding{
				Kind: FindingOpenAPI, ID: "parameter-became-required:" + method + ":" + route + ":" + key, Path: path,
				Summary: prefix + " made parameter required: " + key,
			})
		}
	}

	beforeBody, beforeBodyOK := stringMap(before["requestBody"])
	afterBody, afterBodyOK := stringMap(after["requestBody"])
	if !beforeBodyOK && afterBodyOK && boolValue(afterBody["required"]) {
		findings = append(findings, Finding{
			Kind: FindingOpenAPI, ID: "required-body-added:" + method + ":" + route, Path: path,
			Summary: prefix + " added required request body",
		})
	}
	if beforeBodyOK && afterBodyOK && canonicalJSON(beforeBody["content"]) != canonicalJSON(afterBody["content"]) {
		findings = append(findings, Finding{
			Kind: FindingOpenAPI, ID: "request-body-changed:" + method + ":" + route, Path: path,
			Summary: prefix + " changed request body contract",
		})
	}
	if beforeBodyOK && !boolValue(beforeBody["required"]) && afterBodyOK && boolValue(afterBody["required"]) {
		findings = append(findings, Finding{
			Kind: FindingOpenAPI, ID: "request-body-became-required:" + method + ":" + route, Path: path,
			Summary: prefix + " made request body required",
		})
	}
	return findings
}

func parameterIndex(raw any) map[string]map[string]any {
	values, ok := raw.([]any)
	if !ok {
		return map[string]map[string]any{}
	}
	out := map[string]map[string]any{}
	for _, rawParam := range values {
		param, ok := stringMap(rawParam)
		if !ok {
			continue
		}
		name := strings.TrimSpace(stringValue(param["name"]))
		in := strings.TrimSpace(stringValue(param["in"]))
		if name == "" || in == "" {
			continue
		}
		out[in+":"+name] = param
	}
	return out
}

type architecturePolicy struct {
	Version int                `yaml:"version" json:"version"`
	Rules   []architectureRule `yaml:"rules" json:"rules"`
}

type architectureRule struct {
	ID               string   `yaml:"id" json:"id"`
	Paths            []string `yaml:"paths" json:"paths"`
	ForbiddenImports []string `yaml:"forbidden_imports" json:"forbidden_imports"`
}

type architectureViolation struct {
	ID      string
	Path    string
	Import  string
	RuleID  string
	Summary string
}

func verifyArchitecture(originalRoot, stagedRoot string) ([]Finding, error) {
	policyPath, found, err := findArchitecturePolicy(stagedRoot)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	policy, err := loadArchitecturePolicy(policyPath)
	if err != nil {
		return nil, err
	}
	stagedViolations, err := scanArchitectureViolations(stagedRoot, policy)
	if err != nil {
		return nil, err
	}

	originalPolicyPath, hasOriginalPolicy, err := findArchitecturePolicy(originalRoot)
	if err != nil {
		return nil, err
	}
	originalViolations := map[string]bool{}
	if hasOriginalPolicy {
		originalPolicy, loadErr := loadArchitecturePolicy(originalPolicyPath)
		if loadErr != nil {
			return nil, loadErr
		}
		values, scanErr := scanArchitectureViolations(originalRoot, originalPolicy)
		if scanErr != nil {
			return nil, scanErr
		}
		for _, value := range values {
			originalViolations[value.ID] = true
		}
	}

	var findings []Finding
	for _, violation := range stagedViolations {
		if originalViolations[violation.ID] {
			continue
		}
		findings = append(findings, Finding{
			Kind: FindingArchitecture,
			ID: violation.ID,
			Path: violation.Path,
			Summary: violation.Summary,
		})
	}
	return findings, nil
}

// findArchitecturePolicy detects policy files without following file or
// parent-directory symlinks. A symlink may not turn a policy into an
// apparently missing file or redirect the read outside the project.
func findArchitecturePolicy(root string) (string, bool, error) {
	policyDir := filepath.Join(root, ".ai-engineering-os")
	info, err := os.Lstat(policyDir)
	if os.IsNotExist(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", false, errors.New("architecture policy directory must not be a symbolic link or special file")
	}
	var selected string
	for _, name := range []string{"architecture.yaml", "architecture.yml"} {
		relative := filepath.Join(".ai-engineering-os", name)
		path := filepath.Join(root, relative)
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", false, err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return "", false, errors.New("architecture policy must be a regular file, not a symbolic link")
		}
		if selected != "" {
			return "", false, errors.New("multiple architecture policy files are ambiguous")
		}
		selected = path
	}
	if selected != "" {
		return selected, true, nil
	}
	return "", false, nil
}

func loadArchitecturePolicy(path string) (architecturePolicy, error) {
	data, err := readStableContractFile(path)
	if err != nil {
		return architecturePolicy{}, err
	}
	var value architecturePolicy
	if err := yaml.Unmarshal(data, &value); err != nil {
		return architecturePolicy{}, err
	}
	if value.Version != 1 {
		return architecturePolicy{}, fmt.Errorf("unsupported architecture policy version: %d", value.Version)
	}
	if len(value.Rules) == 0 {
		return architecturePolicy{}, errors.New("architecture policy requires rules")
	}
	seen := map[string]bool{}
	for index, rule := range value.Rules {
		rule.ID = strings.TrimSpace(rule.ID)
		if rule.ID == "" {
			return architecturePolicy{}, fmt.Errorf("architecture rule %d requires id", index)
		}
		if seen[rule.ID] {
			return architecturePolicy{}, fmt.Errorf("duplicate architecture rule id: %s", rule.ID)
		}
		seen[rule.ID] = true
		if len(rule.Paths) == 0 || len(rule.ForbiddenImports) == 0 {
			return architecturePolicy{}, fmt.Errorf("architecture rule %s requires paths and forbidden_imports", rule.ID)
		}
	}
	return value, nil
}

func scanArchitectureViolations(root string, policy architecturePolicy) ([]architectureViolation, error) {
	var values []architectureViolation
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			// WalkDir does not follow symlinked directories. Reject them so
			// architecture policy cannot silently skip an aliased source tree.
			info, statErr := os.Stat(path)
			if statErr != nil {
				return fmt.Errorf("inspect architecture symlink %s: %w", path, statErr)
			}
			if info.IsDir() {
				return fmt.Errorf("architecture directory must not be a symbolic link: %s", path)
			}
		}
		if entry.IsDir() {
			name := entry.Name()
			if name == ".git" || name == "node_modules" || name == "vendor" || name == "build" || name == "dist" {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".go" && ext != ".java" && ext != ".ts" && ext != ".tsx" && ext != ".js" && ext != ".jsx" {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if err := verifyContractFilePath(root, relative); err != nil {
			return fmt.Errorf("inspect architecture source %s: %w", relative, err)
		}
		data, err := readStableContractFile(path)
		if err != nil {
			return fmt.Errorf("read architecture source %s: %w", relative, err)
		}
		// A malformed Go import declaration must not bypass forbidden-import
		// checks. Only files covered by an architecture rule are gated.
		if ext == ".go" {
			covered := false
			for _, rule := range policy.Rules {
				if matchesAnyPath(relative, rule.Paths) {
					covered = true
					break
				}
			}
			if covered {
				if _, parseErr := parser.ParseFile(token.NewFileSet(), relative, data, parser.ImportsOnly); parseErr != nil {
					return fmt.Errorf("parse architecture Go imports %s: %w", relative, parseErr)
				}
			}
		}
		imports := extractImports(string(data), ext)
		for _, rule := range policy.Rules {
			if !matchesAnyPath(relative, rule.Paths) {
				continue
			}
			for _, imported := range imports {
				for _, forbidden := range rule.ForbiddenImports {
					forbidden = strings.TrimSpace(forbidden)
					if forbidden == "" || !strings.Contains(imported, forbidden) {
						continue
					}
					id := rule.ID + ":" + relative + ":" + imported
					values = append(values, architectureViolation{
						ID: id, RuleID: rule.ID, Path: relative, Import: imported,
						Summary: fmt.Sprintf("architecture rule %s forbids import %q", rule.ID, imported),
					})
				}
			}
		}
		return nil
	})
	sort.Slice(values, func(i, j int) bool { return values[i].ID < values[j].ID })
	return values, err
}

var (
	javaImportPattern   = regexp.MustCompile(`(?m)^\s*import\s+(?:static\s+)?([^;]+);`)
	jsImportPattern     = regexp.MustCompile(`(?m)(?:from\s+|require\s*\(\s*)["']([^"']+)["']`)
	sideEffectJSPattern = regexp.MustCompile(`(?m)^\s*import\s+["']([^"']+)["']`)
	dynamicJSPattern    = regexp.MustCompile(`\bimport\s*\(\s*(?:(?:/\*[\s\S]*?\*/|//[^\r\n]*(?:\r?\n|$))\s*)*["']([^"']+)["']\s*(?:(?:/\*[\s\S]*?\*/|//[^\r\n]*(?:\r?\n|$))\s*)*\)`)
)

func extractImports(source, ext string) []string {
	seen := map[string]bool{}
	addMatches := func(pattern *regexp.Regexp) {
		for _, match := range pattern.FindAllStringSubmatch(source, -1) {
			for i := 1; i < len(match); i++ {
				value := strings.TrimSpace(match[i])
				if value != "" {
					seen[value] = true
					break
				}
			}
		}
	}
	switch ext {
	case ".go":
		file, err := parser.ParseFile(token.NewFileSet(), "source.go", source, parser.ImportsOnly)
		if err == nil {
			for _, spec := range file.Imports {
				value, unquoteErr := strconv.Unquote(spec.Path.Value)
				if unquoteErr == nil && strings.TrimSpace(value) != "" {
					seen[value] = true
				}
			}
		}
	case ".java":
		addMatches(javaImportPattern)
	default:
		addMatches(jsImportPattern)
		addMatches(sideEffectJSPattern)
		addMatches(dynamicJSPattern)
	}
	out := make([]string, 0, len(seen))
	for value := range seen {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func matchesAnyPath(path string, patterns []string) bool {
	for _, pattern := range patterns {
		pattern = filepath.ToSlash(strings.TrimSpace(pattern))
		if pattern == "" {
			continue
		}
		if ok, _ := filepath.Match(pattern, path); ok {
			return true
		}
		if strings.HasSuffix(pattern, "/**") {
			prefix := strings.TrimSuffix(pattern, "**")
			if strings.HasPrefix(path, prefix) {
				return true
			}
		}
	}
	return false
}

func stringMap(value any) (map[string]any, bool) {
	if value == nil {
		return nil, false
	}
	if typed, ok := value.(map[string]any); ok {
		return typed, true
	}
	return nil, false
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	if typed, ok := value.(string); ok {
		return typed
	}
	return fmt.Sprint(value)
}

func boolValue(value any) bool {
	typed, _ := value.(bool)
	return typed
}

func canonicalJSON(value any) string {
	data, _ := json.Marshal(value)
	return string(data)
}
