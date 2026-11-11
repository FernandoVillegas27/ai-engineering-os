package contractverify

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, root, relative, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

const openAPIBase = `openapi: 3.0.3
paths:
  /users/{id}:
    get:
      operationId: getUser
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: string
      responses:
        "200":
          description: ok
          content:
            application/json:
              schema:
                type: object
                properties:
                  id:
                    type: string
`

func TestVerifyOpenAPIRemovedOperationBlocks(t *testing.T) {
	original := t.TempDir()
	staged := t.TempDir()
	writeFile(t, original, "openapi.yaml", openAPIBase)
	writeFile(t, staged, "openapi.yaml", `openapi: 3.0.3
paths:
  /users/{id}: {}
`)

	result, err := Verify(original, staged)
	if err != nil {
		t.Fatal(err)
	}
	if result.Passed || len(result.Findings) != 1 {
		t.Fatalf("result=%#v", result)
	}
	if !strings.Contains(result.Findings[0].ID, "operation-removed") {
		t.Fatalf("finding=%#v", result.Findings[0])
	}
}

func TestVerifyOpenAPIAddingOptionalParameterPasses(t *testing.T) {
	original := t.TempDir()
	staged := t.TempDir()
	writeFile(t, original, "contracts/openapi.yaml", openAPIBase)
	writeFile(t, staged, "contracts/openapi.yaml", `openapi: 3.0.3
paths:
  /users/{id}:
    get:
      operationId: getUser
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: string
        - name: trace
          in: query
          required: false
          schema:
            type: string
      responses:
        "200":
          description: ok
          content:
            application/json:
              schema:
                type: object
                properties:
                  id:
                    type: string
`)

	result, err := Verify(original, staged)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Passed {
		t.Fatalf("result=%#v", result)
	}
}

func TestVerifyOpenAPIResponseChangeBlocks(t *testing.T) {
	original := t.TempDir()
	staged := t.TempDir()
	writeFile(t, original, "swagger.yml", openAPIBase)
	writeFile(t, staged, "swagger.yml", strings.Replace(openAPIBase, "type: string", "type: integer", 2))

	result, err := Verify(original, staged)
	if err != nil {
		t.Fatal(err)
	}
	if result.Passed {
		t.Fatalf("result=%#v", result)
	}
}

func TestArchitecturePolicyBlocksOnlyNewViolations(t *testing.T) {
	original := t.TempDir()
	staged := t.TempDir()
	policy := `version: 1
rules:
  - id: core-must-not-import-integrations
    paths:
      - "core/**"
    forbidden_imports:
      - "/integrations/"
`
	for _, root := range []string{original, staged} {
		writeFile(t, root, ".ai-engineering-os/architecture.yaml", policy)
		writeFile(t, root, "core/legacy.go", `package core
import "example.com/project/integrations/legacy"
`)
	}
	writeFile(t, original, "core/new.go", "package core\n")
	writeFile(t, staged, "core/new.go", `package core
import "example.com/project/integrations/new"
`)

	result, err := Verify(original, staged)
	if err != nil {
		t.Fatal(err)
	}
	if result.Passed || len(result.Findings) != 1 {
		t.Fatalf("result=%#v", result)
	}
	if result.Findings[0].Path != "core/new.go" {
		t.Fatalf("finding=%#v", result.Findings[0])
	}
}

func TestArchitecturePolicySupportsJavaAndTypeScriptImports(t *testing.T) {
	original := t.TempDir()
	staged := t.TempDir()
	policy := `version: 1
rules:
  - id: domain-boundary
    paths: ["src/**"]
    forbidden_imports: ["infra"]
`
	for _, root := range []string{original, staged} {
		writeFile(t, root, ".ai-engineering-os/architecture.yaml", policy)
	}
	writeFile(t, staged, "src/App.java", "import com.example.infra.Repository;\nclass App {}\n")
	writeFile(t, staged, "src/app.ts", "import { api } from './infra/api';\n")

	result, err := Verify(original, staged)
	if err != nil {
		t.Fatal(err)
	}
	if result.Passed || len(result.Findings) != 2 {
		t.Fatalf("result=%#v", result)
	}
}


func TestVerifyOpenAPIRejectsStagedSymlinkFile(t *testing.T) {
	original := t.TempDir()
	staged := t.TempDir()
	outside := t.TempDir()
	writeFile(t, original, "contracts/openapi.yaml", openAPIBase)
	writeFile(t, outside, "external.yaml", openAPIBase)
	if err := os.MkdirAll(filepath.Join(staged, "contracts"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "external.yaml"), filepath.Join(staged, "contracts", "openapi.yaml")); err != nil {
		if os.IsPermission(err) {
			t.Skip("symlink creation not permitted")
		}
		t.Fatal(err)
	}
	if _, err := Verify(original, staged); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("staged OpenAPI symlink must block verification, got %v", err)
	}
}

func TestVerifyOpenAPIRejectsStagedSymlinkDirectory(t *testing.T) {
	original := t.TempDir()
	staged := t.TempDir()
	outside := t.TempDir()
	writeFile(t, original, "contracts/openapi.yaml", openAPIBase)
	writeFile(t, outside, "openapi.yaml", openAPIBase)
	if err := os.Symlink(outside, filepath.Join(staged, "contracts")); err != nil {
		if os.IsPermission(err) {
			t.Skip("symlink creation not permitted")
		}
		t.Fatal(err)
	}
	if _, err := Verify(original, staged); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("staged OpenAPI parent symlink must block verification, got %v", err)
	}
}

func TestVerifyOpenAPIRejectsOriginalSymlinkFile(t *testing.T) {
	original := t.TempDir()
	staged := t.TempDir()
	outside := t.TempDir()
	writeFile(t, outside, "openapi.yaml", openAPIBase)
	if err := os.Symlink(filepath.Join(outside, "openapi.yaml"), filepath.Join(original, "openapi.yaml")); err != nil {
		if os.IsPermission(err) {
			t.Skip("symlink creation not permitted")
		}
		t.Fatal(err)
	}
	if _, err := Verify(original, staged); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("original OpenAPI symlink must block verification, got %v", err)
	}
}


func TestArchitectureRejectsStagedPolicyFileSymlink(t *testing.T) {
	original, staged, outside := t.TempDir(), t.TempDir(), t.TempDir()
	policy := "version: 1\nrules:\n  - id: boundary\n    paths: [\"src/**\"]\n    forbidden_imports: [\"infra\"]\n"
	writeFile(t, outside, "architecture.yaml", policy)
	if err := os.MkdirAll(filepath.Join(staged, ".ai-engineering-os"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "architecture.yaml"), filepath.Join(staged, ".ai-engineering-os", "architecture.yaml")); err != nil {
		if os.IsPermission(err) {
			t.Skip("symlinks are not permitted")
		}
		t.Fatal(err)
	}
	if _, err := Verify(original, staged); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("policy symlink must be rejected, got %v", err)
	}
}

func TestArchitectureRejectsStagedPolicyDirectorySymlink(t *testing.T) {
	original, staged, outside := t.TempDir(), t.TempDir(), t.TempDir()
	writeFile(t, outside, "architecture.yaml", "version: 1\nrules:\n  - id: boundary\n    paths: [\"src/**\"]\n    forbidden_imports: [\"infra\"]\n")
	if err := os.Symlink(outside, filepath.Join(staged, ".ai-engineering-os")); err != nil {
		if os.IsPermission(err) {
			t.Skip("symlinks are not permitted")
		}
		t.Fatal(err)
	}
	if _, err := Verify(original, staged); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("policy parent symlink must be rejected, got %v", err)
	}
}

func TestArchitectureRejectsSymlinkedSourceBeforeReading(t *testing.T) {
	original, staged, outside := t.TempDir(), t.TempDir(), t.TempDir()
	policy := "version: 1\nrules:\n  - id: boundary\n    paths: [\"src/**\"]\n    forbidden_imports: [\"infra\"]\n"
	writeFile(t, staged, ".ai-engineering-os/architecture.yaml", policy)
	writeFile(t, outside, "external.go", "package src\nimport \"example.com/infra\"\n")
	if err := os.MkdirAll(filepath.Join(staged, "src"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "external.go"), filepath.Join(staged, "src", "external.go")); err != nil {
		if os.IsPermission(err) {
			t.Skip("symlinks are not permitted")
		}
		t.Fatal(err)
	}
	if _, err := Verify(original, staged); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("source symlink must be rejected, got %v", err)
	}
}

func TestArchitecturePolicyYMLStillSupported(t *testing.T) {
	original, staged := t.TempDir(), t.TempDir()
	writeFile(t, staged, ".ai-engineering-os/architecture.yml", "version: 1\nrules:\n  - id: boundary\n    paths: [\"src/**\"]\n    forbidden_imports: [\"infra\"]\n")
	writeFile(t, staged, "src/source.go", "package src\n")
	result, err := Verify(original, staged)
	if err != nil || !result.Passed {
		t.Fatalf("valid .yml policy must still work: result=%#v err=%v", result, err)
	}
}

func TestArchitectureRejectsAmbiguousPolicyFiles(t *testing.T) {
	original, staged := t.TempDir(), t.TempDir()
	policy := "version: 1\nrules:\n  - id: boundary\n    paths: [\"src/**\"]\n    forbidden_imports: [\"infra\"]\n"
	writeFile(t, staged, ".ai-engineering-os/architecture.yaml", policy)
	writeFile(t, staged, ".ai-engineering-os/architecture.yml", policy)
	if _, err := Verify(original, staged); err == nil || !strings.Contains(err.Error(), "multiple architecture policy") {
		t.Fatalf("ambiguous policies must fail closed, got %v", err)
	}
}

func TestArchitectureRejectsAmbiguousOriginalPolicyFiles(t *testing.T) {
	original, staged := t.TempDir(), t.TempDir()
	policy := "version: 1\nrules:\n  - id: boundary\n    paths: [\"src/**\"]\n    forbidden_imports: [\"infra\"]\n"
	for _, root := range []string{original, staged} {
		writeFile(t, root, ".ai-engineering-os/architecture.yaml", policy)
	}
	writeFile(t, original, ".ai-engineering-os/architecture.yml", policy)
	if _, err := Verify(original, staged); err == nil || !strings.Contains(err.Error(), "multiple architecture policy") {
		t.Fatalf("ambiguous original policies must fail closed, got %v", err)
	}
}
