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


func TestBindOperationsNormalizesAndScopesRuntimeAuthority(t *testing.T) {
	tools := registryFixture()
	evidence := BindOperations(
		FromHealth(tools.Tools["engram"], toolhealth.Status{
			Provider: "engram", Available: true, Healthy: true,
		}, true),
		"read", "read", "",
	)
	if !evidence.RuntimeBound || strings.Join(evidence.BoundOperations, ",") != "read" {
		t.Fatalf("evidence=%#v", evidence)
	}
}

func TestValidateSnapshotRejectsUndeclaredOrMissingBoundOperations(t *testing.T) {
	tools := registryFixture()
	err := ValidateSnapshot(tools, Snapshot{
		"engram": {
			ToolID: "engram", Provider: "engram",
			State: StateAvailable, Healthy: true, RuntimeBound: true,
		},
	})
	if err == nil || !strings.Contains(err.Error(), "without bound operations") {
		t.Fatalf("err=%v", err)
	}

	err = ValidateSnapshot(tools, Snapshot{
		"engram": BindOperations(Evidence{
			ToolID: "engram", Provider: "engram",
			State: StateAvailable, Healthy: true,
		}, "write"),
	})
	if err == nil || !strings.Contains(err.Error(), "not declared") {
		t.Fatalf("err=%v", err)
	}
}

func TestResolvePlanRejectsRuntimeBoundExternalWithoutOperationAuthority(t *testing.T) {
	tools := registryFixture()
	_, _, err := ResolvePlan(
		[]string{"engram"},
		nil,
		tools,
		Snapshot{
			"engram": Bound(FromHealth(tools.Tools["engram"], toolhealth.Status{
				Provider: "engram", Available: true, Healthy: true,
			}, true)),
		},
	)
	if err == nil || !strings.Contains(err.Error(), "no bound runtime operations") {
		t.Fatalf("err=%v", err)
	}
}


func TestEffectiveOperationsNativeUsesDeclaredRegistryOperations(t *testing.T) {
	tool := tool("filesystem", "native", "local")
	tool.Operations = []string{"write-files", "read-files", "read-files"}
	got := EffectiveOperations(tool, Evidence{})
	if strings.Join(got, ",") != "read-files,write-files" {
		t.Fatalf("operations=%#v", got)
	}
}

func TestEffectiveOperationsExternalUsesOnlyBoundSubset(t *testing.T) {
	tools := registryFixture()
	tool := tools.Tools["engram"]
	tool.Operations = []string{"read", "write"}
	evidence := BindOperations(
		FromHealth(tool, toolhealth.Status{
			Provider: "engram", Available: true, Healthy: true,
		}, true),
		"read",
	)
	got := EffectiveOperations(tool, evidence)
	if strings.Join(got, ",") != "read" {
		t.Fatalf("operations=%#v", got)
	}
}

func TestEffectiveOperationsExternalRequiresAvailableBoundEvidence(t *testing.T) {
	tools := registryFixture()
	tool := tools.Tools["engram"]
	if got := EffectiveOperations(tool, Unknown(tool, "not verified")); len(got) != 0 {
		t.Fatalf("operations=%#v", got)
	}
}


func TestValidateSnapshotAllowsReadOnlyGitHubOperationsButRejectsApprovalRequiredWrites(t *testing.T) {
	provider := "github"
	check := "github-connection-available"
	github := registry.Tool{
		ID: "github",
		Description: "github",
		Interface: registry.ToolInterface{Type: "connector", Provider: &provider},
		Operations: []string{"read-repository", "read-issues", "create-commit", "create-pull-request"},
		Access: []string{"read", "write", "network"},
		Risk: "high",
		RequiresApproval: true,
		ApprovalRequiredOperations: []string{"create-commit", "create-pull-request"},
		AvailabilityCheck: &check,
		Status: "enabled",
	}
	tools := registry.ToolRegistry{SchemaVersion: 1, Tools: map[string]registry.Tool{"github": github}}

	readOnly := BindOperations(Evidence{
		ToolID: "github", Provider: "github",
		State: StateAvailable, Healthy: true,
	}, "read-repository", "read-issues")
	if err := ValidateSnapshot(tools, Snapshot{"github": readOnly}); err != nil {
		t.Fatalf("read-only binding rejected: %v", err)
	}

	write := BindOperations(Evidence{
		ToolID: "github", Provider: "github",
		State: StateAvailable, Healthy: true,
	}, "create-commit")
	err := ValidateSnapshot(tools, Snapshot{"github": write})
	if err == nil || !strings.Contains(err.Error(), "requires explicit approval") {
		t.Fatalf("err=%v", err)
	}
}
