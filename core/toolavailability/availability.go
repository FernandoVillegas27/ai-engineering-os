package toolavailability

import (
	"fmt"
	"sort"
	"strings"

	"github.com/FernandoVillegas27/ai-engineering-os/core/registry"
	"github.com/FernandoVillegas27/ai-engineering-os/core/toolhealth"
)

type State string

const (
	StateUnknown     State = "unknown"
	StateAvailable   State = "available"
	StateUnavailable State = "unavailable"
)

type Evidence struct {
	ToolID    string                 `json:"tool_id"`
	Provider  string                 `json:"provider,omitempty"`
	State     State                  `json:"state"`
	Healthy      bool                   `json:"healthy"`
	RuntimeBound bool                   `json:"runtime_bound"`
	Failure      toolhealth.Failure     `json:"failure,omitempty"`
	Details   string                 `json:"details,omitempty"`
}

type Snapshot map[string]Evidence

func Native(tool registry.Tool) Evidence {
	return Evidence{
		ToolID:   tool.ID,
		Provider: providerID(tool),
		State:        StateAvailable,
		Healthy:      true,
		RuntimeBound: true,
		Details:      "native tool declared enabled",
	}
}

func FromHealth(tool registry.Tool, health toolhealth.Status, verified bool) Evidence {
	failure := health.Failure
	failure.Detail = toolhealth.Redact(failure.Detail)
	evidence := Evidence{
		ToolID:   tool.ID,
		Provider: providerID(tool),
		Healthy:      health.Healthy,
		RuntimeBound: false,
		Failure:      failure,
		Details:  toolhealth.Redact(strings.TrimSpace(health.Details)),
		State:    StateUnknown,
	}
	if !health.Available || !health.Healthy {
		evidence.State = StateUnavailable
		return evidence
	}
	if verified {
		evidence.State = StateAvailable
	}
	return evidence
}

func Unknown(tool registry.Tool, detail string) Evidence {
	return Evidence{
		ToolID:   tool.ID,
		Provider: providerID(tool),
		State:    StateUnknown,
		Healthy:      false,
		RuntimeBound: false,
		Details:      toolhealth.Redact(strings.TrimSpace(detail)),
	}
}

func ResolvePlan(
	required []string,
	recommended []string,
	tools registry.ToolRegistry,
	snapshot Snapshot,
) ([]string, []string, error) {
	if snapshot == nil {
		return append([]string(nil), required...), append([]string(nil), recommended...), nil
	}

	requiredOut := make([]string, 0, len(required))
	for _, id := range required {
		tool, ok := tools.Tools[id]
		if !ok {
			return nil, nil, fmt.Errorf("required tool %q is missing from registry", id)
		}
		if tool.Interface.Type == "native" {
			if evidence, exists := snapshot[id]; exists && evidence.State == StateUnavailable {
				return nil, nil, fmt.Errorf("required native tool %q is unavailable", id)
			}
			requiredOut = append(requiredOut, id)
			continue
		}
		evidence, exists := snapshot[id]
		if !exists || evidence.State == StateUnknown {
			return nil, nil, fmt.Errorf(
				"required external tool %q has no verified runtime availability evidence",
				id,
			)
		}
		if evidence.State != StateAvailable {
			return nil, nil, fmt.Errorf(
				"required external tool %q is unavailable at runtime",
				id,
			)
		}
		if !evidence.RuntimeBound {
			return nil, nil, fmt.Errorf(
				"required external tool %q is healthy but not bound to the selected execution runtime",
				id,
			)
		}
		requiredOut = append(requiredOut, id)
	}

	recommendedOut := make([]string, 0, len(recommended))
	requiredSet := map[string]bool{}
	for _, id := range requiredOut {
		requiredSet[id] = true
	}
	for _, id := range recommended {
		if requiredSet[id] {
			continue
		}
		tool, ok := tools.Tools[id]
		if !ok || tool.Status != "enabled" {
			continue
		}
		if tool.Interface.Type == "native" {
			if evidence, exists := snapshot[id]; exists && evidence.State == StateUnavailable {
				continue
			}
			recommendedOut = append(recommendedOut, id)
			continue
		}
		evidence, exists := snapshot[id]
		if exists && evidence.State == StateAvailable && evidence.RuntimeBound {
			recommendedOut = append(recommendedOut, id)
		}
	}

	sort.Strings(requiredOut)
	sort.Strings(recommendedOut)
	return requiredOut, recommendedOut, nil
}

func Bound(evidence Evidence) Evidence {
	evidence.RuntimeBound = true
	return evidence
}

func ValidateSnapshot(tools registry.ToolRegistry, snapshot Snapshot) error {
	for id, evidence := range snapshot {
		if id != evidence.ToolID {
			return fmt.Errorf("tool availability key %q does not match evidence id %q", id, evidence.ToolID)
		}
		tool, ok := tools.Tools[id]
		if !ok {
			return fmt.Errorf("tool availability references unknown tool %q", id)
		}
		if evidence.State != StateUnknown &&
			evidence.State != StateAvailable &&
			evidence.State != StateUnavailable {
			return fmt.Errorf("tool %q has unsupported availability state %q", id, evidence.State)
		}
		if evidence.Provider != "" && providerID(tool) != "" && evidence.Provider != providerID(tool) {
			return fmt.Errorf(
				"tool %q availability provider %q does not match registry provider %q",
				id,
				evidence.Provider,
				providerID(tool),
			)
		}
	}
	return nil
}

func providerID(tool registry.Tool) string {
	if tool.Interface.Provider == nil {
		return ""
	}
	return strings.TrimSpace(*tool.Interface.Provider)
}
