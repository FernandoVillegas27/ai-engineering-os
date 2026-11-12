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

func TestArchitectureMalformedGoImportsFailClosed(t *testing.T) {
	original, staged := t.TempDir(), t.TempDir()
	policy := "version: 1\nrules:\n  - id: boundary\n    paths: [\"src/**\"]\n    forbidden_imports: [\"infra\"]\n"
	writeFile(t, staged, ".ai-engineering-os/architecture.yaml", policy)
	writeFile(t, staged, "src/broken.go", "package src\nimport (\n")
	if _, err := Verify(original, staged); err == nil || !strings.Contains(err.Error(), "parse architecture Go imports") {
		t.Fatalf("malformed covered Go imports must fail closed, got %v", err)
	}
}

func TestArchitectureMalformedUncoveredGoImportsAreNotGated(t *testing.T) {
	original, staged := t.TempDir(), t.TempDir()
	policy := "version: 1\nrules:\n  - id: boundary\n    paths: [\"src/**\"]\n    forbidden_imports: [\"infra\"]\n"
	writeFile(t, staged, ".ai-engineering-os/architecture.yaml", policy)
	writeFile(t, staged, "other/broken.go", "package other\nimport (\n")
	result, err := Verify(original, staged)
	if err != nil || !result.Passed {
		t.Fatalf("uncovered files should not be subject to import syntax gate: result=%#v err=%v", result, err)
	}
}

func TestVerifyRejectsSymlinkedProjectRoots(t *testing.T) {
	original, staged := t.TempDir(), t.TempDir()
	linkParent := t.TempDir()
	for _, tc := range []struct {
		name string
		target string
		originalLink bool
	}{
		{name: "original", target: original, originalLink: true},
		{name: "staged", target: staged},
	} {
		t.Run(tc.name, func(t *testing.T) {
			link := filepath.Join(linkParent, tc.name)
			if err := os.Symlink(tc.target, link); err != nil {
				if os.IsPermission(err) {
					t.Skip("symlink creation not permitted")
				}
				t.Fatal(err)
			}
			left, right := original, staged
			if tc.originalLink {
				left = link
			} else {
				right = link
			}
			if _, err := Verify(left, right); err == nil || !strings.Contains(err.Error(), "symbolic link") {
				t.Fatalf("symlinked %s root must fail closed, got %v", tc.name, err)
			}
		})
	}
}

func TestVerifyRejectsNonDirectoryRoot(t *testing.T) {
	original, staged := t.TempDir(), t.TempDir()
	writeFile(t, original, "not-a-directory", "content")
	if _, err := Verify(filepath.Join(original, "not-a-directory"), staged); err == nil || !strings.Contains(err.Error(), "real directory") {
		t.Fatalf("non-directory root must fail closed, got %v", err)
	}
}

func TestVerifyRejectsSymlinkedAncestorDirectory(t *testing.T) {
	realParent := t.TempDir()
	original := filepath.Join(realParent, "original")
	staged := filepath.Join(realParent, "staged")
	if err := os.Mkdir(original, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(staged, 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "parent-link")
	if err := os.Symlink(realParent, link); err != nil {
		if os.IsPermission(err) {
			t.Skip("symlink creation not permitted")
		}
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		left string
		right string
	}{
		{name: "original ancestor", left: filepath.Join(link, "original"), right: staged},
		{name: "staged ancestor", left: original, right: filepath.Join(link, "staged")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Verify(tc.left, tc.right); err == nil || !strings.Contains(err.Error(), "symbolic links") {
				t.Fatalf("symlinked ancestor must fail closed: %v", err)
			}
		})
	}
}

func TestVerifyAllowsOrdinaryNestedRootDirectories(t *testing.T) {
	parent := t.TempDir()
	original := filepath.Join(parent, "nested", "original")
	staged := filepath.Join(parent, "nested", "staged")
	for _, root := range []string{original, staged} {
		if err := os.MkdirAll(root, 0700); err != nil {
			t.Fatal(err)
		}
	}
	result, err := Verify(original, staged)
	if err != nil || !result.Passed {
		t.Fatalf("ordinary nested roots must remain valid: result=%#v err=%v", result, err)
	}
}

func TestVerifyAllowsSymlinkedSystemStyleGrandparent(t *testing.T) {
	// macOS commonly exposes /var through a system symlink. Only the
	// workspace root and its direct parent are under this guard.
	realParent := t.TempDir()
	nested := filepath.Join(realParent, "workspace")
	for _, name := range []string{"original", "staged"} {
		if err := os.MkdirAll(filepath.Join(nested, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	link := filepath.Join(t.TempDir(), "system-style-link")
	if err := os.Symlink(realParent, link); err != nil {
		if os.IsPermission(err) {
			t.Skip("symlink creation not permitted")
		}
		t.Fatal(err)
	}
	result, err := Verify(filepath.Join(link, "workspace", "original"), filepath.Join(link, "workspace", "staged"))
	if err != nil || !result.Passed {
		t.Fatalf("system-style grandparent symlink must not block ordinary workspaces: result=%#v err=%v", result, err)
	}
}

func TestReadStableContractFileAcceptsRegularFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "openapi.yaml")
	want := []byte("openapi: 3.0.0\npaths: {}\n")
	if err := os.WriteFile(path, want, 0600); err != nil {
		t.Fatal(err)
	}
	got, err := readStableContractFile(path)
	if err != nil || string(got) != string(want) {
		t.Fatalf("regular contract read: got=%q err=%v", got, err)
	}
}

func TestReadStableContractFileRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.yaml")
	if err := os.WriteFile(target, []byte("openapi: 3.0.0"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "openapi.yaml")
	if err := os.Symlink(target, link); err != nil {
		if os.IsPermission(err) {
			t.Skip("symlink creation not permitted")
		}
		t.Fatal(err)
	}
	if _, err := readStableContractFile(link); err == nil {
		t.Fatal("symlinked contract must fail closed")
	}
}

func TestReadStableContractFileRejectsDirectory(t *testing.T) {
	if _, err := readStableContractFile(t.TempDir()); err == nil {
		t.Fatal("directory cannot be read as a contract")
	}
}

func TestLoadArchitecturePolicyRejectsSymlinkedPolicy(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.yaml")
	if err := os.WriteFile(target, []byte("rules: []\n"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "architecture.yaml")
	if err := os.Symlink(target, link); err != nil {
		if os.IsPermission(err) {
			t.Skip("symlink creation not permitted")
		}
		t.Fatal(err)
	}
	if _, err := loadArchitecturePolicy(link); err == nil {
		t.Fatal("architecture policy symlink must fail closed at read boundary")
	}
}

func TestLoadArchitecturePolicyRejectsDirectory(t *testing.T) {
	if _, err := loadArchitecturePolicy(t.TempDir()); err == nil {
		t.Fatal("architecture policy directory must fail closed")
	}
}

func TestScanArchitectureViolationsReadsRegularSource(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "src/main.go", "package main\nimport \"unsafe\"\n")
	policy := architecturePolicy{Rules: []architectureRule{{
		ID: "no-unsafe", Paths: []string{"**/*.go"}, ForbiddenImports: []string{"unsafe"},
	}}}
	violations, err := scanArchitectureViolations(root, policy)
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) != 1 || violations[0].Import != "unsafe" {
		t.Fatalf("expected forbidden import from regular source, got %#v", violations)
	}
}

func TestScanArchitectureViolationsRejectsSymlinkedSource(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "target.go")
	if err := os.WriteFile(target, []byte("package main\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(root, "main.go")); err != nil {
		if os.IsPermission(err) {
			t.Skip("symlink creation not permitted")
		}
		t.Fatal(err)
	}
	if _, err := scanArchitectureViolations(root, architecturePolicy{}); err == nil {
		t.Fatal("symlinked architecture source must fail closed")
	}
}

func TestArchitectureRejectsLiteralDynamicJavaScriptImports(t *testing.T) {
	for _, ext := range []string{".js", ".jsx", ".ts", ".tsx"} {
		t.Run(ext, func(t *testing.T) {
			original, staged := t.TempDir(), t.TempDir()
			policy := "version: 1\nrules:\n  - id: boundary\n    paths: [\"src/**\"]\n    forbidden_imports: [\"infra\"]\n"
			writeFile(t, staged, ".ai-engineering-os/architecture.yaml", policy)
			writeFile(t, staged, "src/module"+ext, "const module = import( \"example.com/infra\" );\n")
			result, err := Verify(original, staged)
			if err != nil {
				t.Fatal(err)
			}
			if result.Passed || len(result.Findings) != 1 || result.Findings[0].Kind != FindingArchitecture {
				t.Fatalf("dynamic import must be gated: %#v", result)
			}
		})
	}
}

func TestArchitectureAllowsNonForbiddenLiteralDynamicImport(t *testing.T) {
	original, staged := t.TempDir(), t.TempDir()
	writeFile(t, staged, ".ai-engineering-os/architecture.yaml", "version: 1\nrules:\n  - id: boundary\n    paths: [\"src/**\"]\n    forbidden_imports: [\"infra\"]\n")
	writeFile(t, staged, "src/module.ts", "const module = import(\"example.com/allowed\");\n")
	result, err := Verify(original, staged)
	if err != nil || !result.Passed {
		t.Fatalf("allowed dynamic import should pass: result=%#v err=%v", result, err)
	}
}

func TestArchitectureRejectsCommentedLiteralDynamicImports(t *testing.T) {
	for _, ext := range []string{".js", ".jsx", ".ts", ".tsx"} {
		t.Run(ext, func(t *testing.T) {
			original, staged := t.TempDir(), t.TempDir()
			policy := "version: 1\nrules:\n  - id: boundary\n    paths: [\"src/**\"]\n    forbidden_imports: [\"infra\"]\n"
			writeFile(t, staged, ".ai-engineering-os/architecture.yaml", policy)
			writeFile(t, staged, "src/module"+ext, "const module = import( /* bundler: chunk\n   split */ \"example.com/infra\" );\n")
			result, err := Verify(original, staged)
			if err != nil {
				t.Fatal(err)
			}
			if result.Passed || len(result.Findings) != 1 || result.Findings[0].Kind != FindingArchitecture {
				t.Fatalf("commented dynamic import must be gated: %#v", result)
			}
		})
	}
}

func TestArchitectureAllowsCommentedNonForbiddenDynamicImport(t *testing.T) {
	original, staged := t.TempDir(), t.TempDir()
	writeFile(t, staged, ".ai-engineering-os/architecture.yaml", "version: 1\nrules:\n  - id: boundary\n    paths: [\"src/**\"]\n    forbidden_imports: [\"infra\"]\n")
	writeFile(t, staged, "src/module.ts", "const module = import(/* chunk */ \"example.com/allowed\");\n")
	result, err := Verify(original, staged)
	if err != nil || !result.Passed {
		t.Fatalf("allowed commented dynamic import should pass: result=%#v err=%v", result, err)
	}
}

func TestArchitectureRejectsConsecutiveCommentedDynamicImports(t *testing.T) {
	for _, ext := range []string{".js", ".jsx", ".ts", ".tsx"} {
		t.Run(ext, func(t *testing.T) {
			original, staged := t.TempDir(), t.TempDir()
			writeFile(t, staged, ".ai-engineering-os/architecture.yaml", "version: 1\nrules:\n  - id: boundary\n    paths: [\"src/**\"]\n    forbidden_imports: [\"infra\"]\n")
			writeFile(t, staged, "src/module"+ext, "const module = import(/* bundler */ /* chunk\n name */ \"example.com/infra\");\n")
			result, err := Verify(original, staged)
			if err != nil {
				t.Fatal(err)
			}
			if result.Passed || len(result.Findings) != 1 || result.Findings[0].Kind != FindingArchitecture {
				t.Fatalf("consecutive comments must not bypass architecture rules: %#v", result)
			}
		})
	}
}

func TestArchitectureAllowsConsecutiveCommentedDynamicImport(t *testing.T) {
	original, staged := t.TempDir(), t.TempDir()
	writeFile(t, staged, ".ai-engineering-os/architecture.yaml", "version: 1\nrules:\n  - id: boundary\n    paths: [\"src/**\"]\n    forbidden_imports: [\"infra\"]\n")
	writeFile(t, staged, "src/module.ts", "const module = import(/* a */ /* b */ \"example.com/allowed\");\n")
	result, err := Verify(original, staged)
	if err != nil || !result.Passed {
		t.Fatalf("allowed import should pass: result=%#v err=%v", result, err)
	}
}

func TestScanArchitectureViolationsRejectsSymlinkedDirectory(t *testing.T) {
	root := t.TempDir()
	target := t.TempDir()
	if err := os.WriteFile(filepath.Join(target, "module.ts"), []byte("import 'example.com/infra'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(root, "src")); err != nil {
		if os.IsPermission(err) {
			t.Skip("symlink creation not permitted")
		}
		t.Fatal(err)
	}
	policy := architecturePolicy{Rules: []architectureRule{{
		ID: "boundary", Paths: []string{"src/**"}, ForbiddenImports: []string{"infra"},
	}}}
	if _, err := scanArchitectureViolations(root, policy); err == nil {
		t.Fatal("symlinked architecture directory must fail closed instead of skipping source files")
	}
}

func TestDiscoverOpenAPIFilesRejectsSymlinkedContractDirectory(t *testing.T) {
	root, target := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(target, "openapi.yaml"), []byte("openapi: 3.0.0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(root, "contracts")); err != nil {
		if os.IsPermission(err) {
			t.Skip("symlink creation not permitted")
		}
		t.Fatal(err)
	}
	if _, err := discoverOpenAPIFiles(root); err == nil {
		t.Fatal("linked contract directory must fail closed rather than hiding OpenAPI files")
	}
}

func TestArchitectureRejectsDynamicImportWithLineComments(t *testing.T) {
	for _, ext := range []string{".js", ".jsx", ".ts", ".tsx"} {
		t.Run(ext, func(t *testing.T) {
			original, staged := t.TempDir(), t.TempDir()
			writeFile(t, staged, ".ai-engineering-os/architecture.yaml", "version: 1\nrules:\n  - id: boundary\n    paths: [\"src/**\"]\n    forbidden_imports: [\"infra\"]\n")
			writeFile(t, staged, "src/module"+ext, "const module = import(// bundler hint\n /* chunk */ // second hint\n \"example.com/infra\");\n")
			result, err := Verify(original, staged)
			if err != nil {
				t.Fatal(err)
			}
			if result.Passed || len(result.Findings) != 1 || result.Findings[0].Kind != FindingArchitecture {
				t.Fatalf("line comments must not bypass architecture policy: %#v", result)
			}
		})
	}
}

func TestArchitectureAllowsDynamicImportWithLineComments(t *testing.T) {
	original, staged := t.TempDir(), t.TempDir()
	writeFile(t, staged, ".ai-engineering-os/architecture.yaml", "version: 1\nrules:\n  - id: boundary\n    paths: [\"src/**\"]\n    forbidden_imports: [\"infra\"]\n")
	writeFile(t, staged, "src/module.ts", "const module = import(// bundler hint\n \"example.com/allowed\");\n")
	result, err := Verify(original, staged)
	if err != nil || !result.Passed {
		t.Fatalf("allowed dynamic import should pass: result=%#v err=%v", result, err)
	}
}

func TestArchitectureRejectsDynamicImportWithTrailingComments(t *testing.T) {
	for _, ext := range []string{".js", ".jsx", ".ts", ".tsx"} {
		t.Run(ext, func(t *testing.T) {
			original, staged := t.TempDir(), t.TempDir()
			writeFile(t, staged, ".ai-engineering-os/architecture.yaml", "version: 1\nrules:\n  - id: boundary\n    paths: [\"src/**\"]\n    forbidden_imports: [\"infra\"]\n")
			writeFile(t, staged, "src/module"+ext, "const module = import(\"example.com/infra\" /* chunk */ // trailing\n );\n")
			result, err := Verify(original, staged)
			if err != nil {
				t.Fatal(err)
			}
			if result.Passed || len(result.Findings) != 1 || result.Findings[0].Kind != FindingArchitecture {
				t.Fatalf("trailing comments must not bypass architecture policy: %#v", result)
			}
		})
	}
}

func TestArchitectureAllowsDynamicImportWithTrailingComments(t *testing.T) {
	original, staged := t.TempDir(), t.TempDir()
	writeFile(t, staged, ".ai-engineering-os/architecture.yaml", "version: 1\nrules:\n  - id: boundary\n    paths: [\"src/**\"]\n    forbidden_imports: [\"infra\"]\n")
	writeFile(t, staged, "src/module.ts", "const module = import(\"example.com/allowed\" // hint\n );\n")
	result, err := Verify(original, staged)
	if err != nil || !result.Passed {
		t.Fatalf("allowed import should pass: result=%#v err=%v", result, err)
	}
}
