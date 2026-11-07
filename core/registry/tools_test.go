package registry

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeToolsFixture(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tools.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadToolsValidatesExternalHealthMetadata(t *testing.T) {
	path := writeToolsFixture(t, `schema_version: 1
tools:
  docs:
    id: docs
    description: External docs
    category: documentation
    interface:
      type: mcp
      provider: context7
    operations: [query-docs]
    access: [read, network]
    risk: low
    requires_approval: false
    availability_check: context7-available
    status: enabled
`)
	registry, err := LoadTools(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(registry.Tools) != 1 {
		t.Fatalf("registry=%#v", registry)
	}
}

func TestLoadToolsRejectsIncompleteOrUnsafeMetadata(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{
			name: "mcp-missing-provider",
			body: `schema_version: 1
tools:
  x:
    id: x
    description: x
    category: docs
    interface: {type: mcp}
    operations: [read]
    access: [read]
    risk: low
    availability_check: x-ready
    status: enabled
`,
			want: "requires provider",
		},
		{
			name: "connector-missing-health",
			body: `schema_version: 1
tools:
  x:
    id: x
    description: x
    category: scm
    interface: {type: connector, provider: github}
    operations: [read]
    access: [read, network]
    risk: high
    status: enabled
`,
			want: "requires availability_check",
		},
		{
			name: "invalid-access",
			body: `schema_version: 1
tools:
  x:
    id: x
    description: x
    category: docs
    interface: {type: native}
    operations: [read]
    access: [root]
    risk: low
    status: enabled
`,
			want: "unsupported access mode",
		},
		{
			name: "invalid-risk",
			body: `schema_version: 1
tools:
  x:
    id: x
    description: x
    category: docs
    interface: {type: native}
    operations: [read]
    access: [read]
    risk: extreme
    status: enabled
`,
			want: "risk must be",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := LoadTools(writeToolsFixture(t, tc.body))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err=%v want contains %q", err, tc.want)
			}
		})
	}
}


func TestOperationRequiresApprovalSupportsScopedAndLegacyPolicies(t *testing.T) {
	scoped := Tool{
		Operations: []string{"read-repository", "create-commit"},
		RequiresApproval: true,
		ApprovalRequiredOperations: []string{"create-commit"},
	}
	if OperationRequiresApproval(scoped, "read-repository") {
		t.Fatal("read-repository should not require approval under scoped policy")
	}
	if !OperationRequiresApproval(scoped, "create-commit") {
		t.Fatal("create-commit must require approval")
	}

	legacy := Tool{
		Operations: []string{"read"},
		RequiresApproval: true,
	}
	if !OperationRequiresApproval(legacy, "read") {
		t.Fatal("legacy whole-tool approval policy must remain compatible")
	}
}

func TestLoadToolsRejectsUndeclaredApprovalRequiredOperation(t *testing.T) {
	path := writeToolsFixture(t, `schema_version: 1
tools:
  github:
    id: github
    description: GitHub
    category: version-control
    interface: {type: connector, provider: github}
    operations: [read-repository]
    access: [read, network]
    risk: high
    requires_approval: true
    approval_required_operations: [create-commit]
    availability_check: github-connection-available
    status: enabled
`)
	_, err := LoadTools(path)
	if err == nil || !strings.Contains(err.Error(), "is not declared in operations") {
		t.Fatalf("err=%v", err)
	}
}
