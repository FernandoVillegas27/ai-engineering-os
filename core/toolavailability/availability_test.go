package toolavailability

import (
	"strings"
	"testing"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/registry"
	"github.com/FernandoVillegas27/ai-engineering-os/core/toolapproval"
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


func TestValidateSnapshotPlaywrightAllowsOnlyNonApprovalInspectionWithoutApprovalEvidence(t *testing.T) {
	provider := "playwright"
	check := "playwright-available"
	playwright := registry.Tool{
		ID: "playwright",
		Description: "browser",
		Interface: registry.ToolInterface{Type: "mcp", Provider: &provider},
		Operations: []string{"inspect-ui", "navigate-browser", "interact-browser", "run-browser-checks"},
		Access: []string{"read", "execute", "network"},
		Risk: "high",
		RequiresApproval: true,
		ApprovalRequiredOperations: []string{"navigate-browser", "interact-browser", "run-browser-checks"},
		AvailabilityCheck: &check,
		Status: "enabled",
	}
	tools := registry.ToolRegistry{SchemaVersion: 1, Tools: map[string]registry.Tool{"playwright": playwright}}

	inspect := BindOperations(Evidence{
		ToolID: "playwright", Provider: "playwright",
		State: StateAvailable, Healthy: true,
	}, "inspect-ui")
	if err := ValidateSnapshot(tools, Snapshot{"playwright": inspect}); err != nil {
		t.Fatalf("inspect-ui binding rejected: %v", err)
	}

	for _, operation := range []string{"navigate-browser", "interact-browser", "run-browser-checks"} {
		evidence := BindOperations(Evidence{
			ToolID: "playwright", Provider: "playwright",
			State: StateAvailable, Healthy: true,
		}, operation)
		err := ValidateSnapshot(tools, Snapshot{"playwright": evidence})
		if err == nil || !strings.Contains(err.Error(), "requires explicit approval") {
			t.Fatalf("%s err=%v", operation, err)
		}
	}
}


func TestValidateSnapshotWithApprovalsAllowsOnlyExactGrantedOperation(t *testing.T) {
	provider := "github"
	check := "github-connection-available"
	github := registry.Tool{
		ID: "github",
		Description: "github",
		Interface: registry.ToolInterface{Type: "connector", Provider: &provider},
		Operations: []string{"read-repository", "create-commit", "create-pull-request"},
		Access: []string{"read", "write", "network"},
		Risk: "high",
		RequiresApproval: true,
		ApprovalRequiredOperations: []string{"create-commit", "create-pull-request"},
		AvailabilityCheck: &check,
		Status: "enabled",
	}
	tools := registry.ToolRegistry{SchemaVersion: 1, Tools: map[string]registry.Tool{"github": github}}
	evidence := BindOperations(Evidence{
		ToolID: "github", Provider: "github",
		State: StateAvailable, Healthy: true,
	}, "create-pull-request")

	now := time.Date(2026, 11, 7, 18, 0, 0, 0, time.UTC)
	approval, err := toolapproval.New(toolapproval.Request{
		RunID: "run-1", TaskID: "task-1", ToolID: "github",
		Operation: "create-pull-request", Status: toolapproval.StatusGranted, Actor: "human",
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateSnapshotWithApprovals(
		tools, Snapshot{"github": evidence}, "run-1", "task-1",
		[]toolapproval.Evidence{approval}, now,
	); err != nil {
		t.Fatalf("exact approval rejected: %v", err)
	}

	for _, tc := range []struct {
		name, runID, taskID string
		approvals []toolapproval.Evidence
	}{
		{name: "wrong-run", runID: "run-2", taskID: "task-1", approvals: []toolapproval.Evidence{approval}},
		{name: "wrong-task", runID: "run-1", taskID: "task-2", approvals: []toolapproval.Evidence{approval}},
		{name: "missing", runID: "run-1", taskID: "task-1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateSnapshotWithApprovals(
				tools, Snapshot{"github": evidence}, tc.runID, tc.taskID, tc.approvals, now,
			)
			if err == nil || !strings.Contains(err.Error(), "requires explicit approval") {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

func TestValidateSnapshotWithApprovalsRejectsRejectedAndExpiredEvidence(t *testing.T) {
	provider := "playwright"
	check := "playwright-available"
	playwright := registry.Tool{
		ID: "playwright",
		Description: "browser",
		Interface: registry.ToolInterface{Type: "mcp", Provider: &provider},
		Operations: []string{"inspect-ui", "navigate-browser"},
		Access: []string{"read", "execute", "network"},
		Risk: "high",
		RequiresApproval: true,
		ApprovalRequiredOperations: []string{"navigate-browser"},
		AvailabilityCheck: &check,
		Status: "enabled",
	}
	tools := registry.ToolRegistry{SchemaVersion: 1, Tools: map[string]registry.Tool{"playwright": playwright}}
	bound := BindOperations(Evidence{
		ToolID: "playwright", Provider: "playwright",
		State: StateAvailable, Healthy: true,
	}, "navigate-browser")
	now := time.Date(2026, 11, 7, 18, 0, 0, 0, time.UTC)

	rejected, err := toolapproval.New(toolapproval.Request{
		RunID: "run-1", TaskID: "task-1", ToolID: "playwright",
		Operation: "navigate-browser", Status: toolapproval.StatusRejected, Actor: "human",
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateSnapshotWithApprovals(
		tools, Snapshot{"playwright": bound}, "run-1", "task-1",
		[]toolapproval.Evidence{rejected}, now,
	); err == nil {
		t.Fatal("rejected decision must not grant authority")
	}

	expires := now.Add(time.Minute)
	granted, err := toolapproval.New(toolapproval.Request{
		RunID: "run-1", TaskID: "task-1", ToolID: "playwright",
		Operation: "navigate-browser", Status: toolapproval.StatusGranted, Actor: "human",
		ExpiresAt: &expires,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateSnapshotWithApprovals(
		tools, Snapshot{"playwright": bound}, "run-1", "task-1",
		[]toolapproval.Evidence{granted}, expires.Add(time.Second),
	); err == nil {
		t.Fatal("expired decision must fail closed")
	}
}
