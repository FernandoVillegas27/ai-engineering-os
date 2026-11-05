package contractverify

import (
	"encoding/json"
	"errors"
	"fmt"
	"go/parser"
	"go/token"
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

func verifyOpenAPIContracts(originalRoot, stagedRoot string) ([]Finding, error) {
	originalFiles, err := discoverOpenAPIFiles(originalRoot)
	if err != nil {
		return nil, err
	}
	var findings []Finding
	for _, relative := range originalFiles {
		before, err := loadOpenAPIDocument(filepath.Join(originalRoot, relative))
		if err != nil {
			return nil, fmt.Errorf("load original OpenAPI %s: %w", relative, err)
		}
		stagedPath := filepath.Join(stagedRoot, relative)
		if _, err := os.Stat(stagedPath); os.IsNotExist(err) {
			findings = append(findings, Finding{
				Kind: FindingOpenAPI, ID: "contract-removed", Path: relative,
				Summary: "existing OpenAPI contract file was removed",
			})
			continue
		} else if err != nil {
			return nil, err
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

func loadOpenAPIDocument(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
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
	policyPath := filepath.Join(stagedRoot, ".ai-engineering-os", "architecture.yaml")
	if _, err := os.Stat(policyPath); os.IsNotExist(err) {
		policyPath = filepath.Join(stagedRoot, ".ai-engineering-os", "architecture.yml")
	}
	if _, err := os.Stat(policyPath); os.IsNotExist(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	policy, err := loadArchitecturePolicy(policyPath)
	if err != nil {
		return nil, err
	}
	stagedViolations, err := scanArchitectureViolations(stagedRoot, policy)
	if err != nil {
		return nil, err
	}

	originalPolicyPath := filepath.Join(originalRoot, ".ai-engineering-os", "architecture.yaml")
	if _, err := os.Stat(originalPolicyPath); os.IsNotExist(err) {
		originalPolicyPath = filepath.Join(originalRoot, ".ai-engineering-os", "architecture.yml")
	}
	originalViolations := map[string]bool{}
	if _, err := os.Stat(originalPolicyPath); err == nil {
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

func loadArchitecturePolicy(path string) (architecturePolicy, error) {
	data, err := os.ReadFile(path)
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
		data, err := os.ReadFile(path)
		if err != nil {
			return err
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
