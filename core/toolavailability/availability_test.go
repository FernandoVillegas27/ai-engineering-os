package toolavailability

import (
	"strings"
	"testing"

	"github.com/FernandoVillegas27/ai-engineering-os/core/registry"
	"github.com/FernandoVillegas27/ai-engineering-os/core/toolhealth"
)

func tool(id, interfaceType, provider string) registry.Tool {
	var providerValue *string
	if provider != "" {
		value := provider
		providerValue = &value
	}
	return registry.Tool{
		ID: id,
		Description: id,
		Interface: registry.ToolInterface{Type: interfaceType, Provider: providerValue},
		Operations: []string{"read"},
		Access: []string{"read"},
		Risk: "low",
		Status: "enabled",
	}
}

func registryFixture() registry.ToolRegistry {
	return registry.ToolRegistry{SchemaVersion: 1, Tools: map[string]registry.Tool{
		"filesystem": tool("filesystem", "native", "local-runtime"),
		"engram": tool("engram", "mcp", "engram"),
		"context7": tool("context7", "mcp", "context7"),
		"github": tool("github", "connector", "github"),
	}}
}

func TestResolvePlanRequiresVerifiedExternalEvidence(t *testing.T) {
	tools := registryFixture()
	_, _, err := ResolvePlan(
		[]string{"filesystem", "github"},
		nil,
		tools,
		Snapshot{"filesystem": Native(tools.Tools["filesystem"])},
	)
	if err == nil || !strings.Contains(err.Error(), "no verified runtime availability evidence") {
		t.Fatalf("err=%v", err)
	}
}

func TestResolvePlanAllowsVerifiedExternalAndOmitsUnknownOptional(t *testing.T) {
	tools := registryFixture()
	required, recommended, err := ResolvePlan(
		[]string{"filesystem", "engram"},
		[]string{"context7", "github"},
		tools,
		Snapshot{
			"filesystem": Native(tools.Tools["filesystem"]),
			"engram": BindOperations(FromHealth(tools.Tools["engram"], toolhealth.Status{
				Provider: "engram", Available: true, Healthy: true,
			}, true), "read"),
			"context7": Unknown(tools.Tools["context7"], "remote host connectivity not verified"),
			"github": Unknown(tools.Tools["github"], "connector status unavailable"),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(required, ",") != "engram,filesystem" {
		t.Fatalf("required=%#v", required)
	}
	if len(recommended) != 0 {
		t.Fatalf("recommended=%#v", recommended)
	}
}

func TestResolvePlanKeepsOnlyAvailableOptionalExternalTools(t *testing.T) {
	tools := registryFixture()
	_, recommended, err := ResolvePlan(
		[]string{"filesystem"},
		[]string{"engram", "context7", "github"},
		tools,
		Snapshot{
			"filesystem": Native(tools.Tools["filesystem"]),
			"engram": BindOperations(FromHealth(tools.Tools["engram"], toolhealth.Status{
				Provider: "engram", Available: true, Healthy: true,
			}, true), "read"),
			"context7": Unknown(tools.Tools["context7"], "launcher only"),
			"github": Unknown(tools.Tools["github"], "not connected"),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(recommended, ",") != "engram" {
		t.Fatalf("recommended=%#v", recommended)
	}
}

func TestFromHealthUnavailableFailsClosedAndRedactsDetails(t *testing.T) {
	tools := registryFixture()
	evidence := FromHealth(tools.Tools["engram"], toolhealth.Status{
		Provider: "engram",
		Available: true,
		Healthy: false,
		Details: "token=super-secret",
		Failure: toolhealth.Failure{Kind: toolhealth.FailureAuth, Detail: "token=super-secret"},
	}, true)
	if evidence.State != StateUnavailable {
		t.Fatalf("evidence=%#v", evidence)
	}
	if strings.Contains(evidence.Details, "super-secret") {
		t.Fatalf("details leaked secret: %q", evidence.Details)
	}
}

func TestValidateSnapshotRejectsProviderMismatch(t *testing.T) {
	tools := registryFixture()
	err := ValidateSnapshot(tools, Snapshot{
		"engram": {ToolID: "engram", Provider: "other", State: StateAvailable},
	})
	if err == nil || !strings.Contains(err.Error(), "does not match registry provider") {
		t.Fatalf("err=%v", err)
	}
}

func TestNilSnapshotPreservesDeclarativePlan(t *testing.T) {
	tools := registryFixture()
	required, recommended, err := ResolvePlan(
		[]string{"filesystem"},
		[]string{"github"},
		tools,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(required, ",") != "filesystem" || strings.Join(recommended, ",") != "github" {
		t.Fatalf("required=%#v recommended=%#v", required, recommended)
	}
}


func TestResolvePlanRejectsHealthyExternalToolWithoutRuntimeBinding(t *testing.T) {
	tools := registryFixture()
	_, _, err := ResolvePlan(
		[]string{"engram"},
		nil,
		tools,
		Snapshot{
			"engram": FromHealth(tools.Tools["engram"], toolhealth.Status{
				Provider: "engram", Available: true, Healthy: true,
			}, true),
		},
	)
	if err == nil || !strings.Contains(err.Error(), "not bound to the selected execution runtime") {
		t.Fatalf("err=%v", err)
	}
}


func TestNativeBindsAllDeclaredOperations(t *testing.T) {
	tool := tool("filesystem", "native", "local")
	tool.Operations = []string{"write-files", "read-files", "read-files"}
	evidence := Native(tool)
	if !evidence.RuntimeBound || strings.Join(evidence.BoundOperations, ",") != "read-files,write-files" {
		t.Fatalf("evidence=%#v", evidence)
	}
}

func TestBindOperationsIsExplicitSortedAndDeduplicated(t *testing.T) {
	tools := registryFixture()
	evidence := BindOperations(
		FromHealth(tools.Tools["engram"], toolhealth.Status{
			Provider: "engram", Available: true, Healthy: true,
		}, true),
		"write", "read", "write",
	)
	if !evidence.RuntimeBound || strings.Join(evidence.BoundOperations, ",") != "read,write" {
		t.Fatalf("evidence=%#v", evidence)
	}
}

func TestValidateSnapshotRejectsUndeclaredBoundOperation(t *testing.T) {
	tools := registryFixture()
	evidence := BindOperations(
		FromHealth(tools.Tools["engram"], toolhealth.Status{
			Provider: "engram", Available: true, Healthy: true,
		}, true),
		"delete-everything",
	)
	err := ValidateSnapshot(tools, Snapshot{"engram": evidence})
	if err == nil || !strings.Contains(err.Error(), "undeclared operation") {
		t.Fatalf("err=%v", err)
	}
}

func TestValidateSnapshotRejectsRuntimeBoundWithoutOperations(t *testing.T) {
	tools := registryFixture()
	evidence := FromHealth(tools.Tools["engram"], toolhealth.Status{
		Provider: "engram", Available: true, Healthy: true,
	}, true)
	evidence.RuntimeBound = true
	err := ValidateSnapshot(tools, Snapshot{"engram": evidence})
	if err == nil || !strings.Contains(err.Error(), "runtime_bound must match") {
		t.Fatalf("err=%v", err)
	}
}

func TestOperationsForExternalReturnsOnlyExplicitBinding(t *testing.T) {
	tools := registryFixture()
	tool := tools.Tools["engram"]
	tool.Operations = []string{"read", "write"}
	evidence := BindOperations(
		FromHealth(tool, toolhealth.Status{Provider: "engram", Available: true, Healthy: true}, true),
		"read",
	)
	if got := strings.Join(OperationsFor(tool, evidence), ","); got != "read" {
		t.Fatalf("operations=%q", got)
	}
}
