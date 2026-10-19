package classifier

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/FernandoVillegas27/ai-engineering-os/core/modelprovider"
	"github.com/FernandoVillegas27/ai-engineering-os/core/orchestration"
	"github.com/FernandoVillegas27/ai-engineering-os/core/registry"
)

type Source string

const (
	SourceDeterministic Source = "deterministic"
	SourceAI            Source = "ai"
)

type Resolution struct {
	Result                Result
	Source                Source
	Provider              string
	ClarificationRequired bool
	ClarificationQuestion string
}

func Resolve(
	ctx context.Context,
	task string,
	capabilities registry.CapabilityRegistry,
	projectContext string,
	provider modelprovider.Provider,
) (Resolution, error) {
	deterministic, err := Classify(task, capabilities)
	if err != nil {
		return Resolution{}, err
	}

	if !deterministic.NeedsAI {
		return Resolution{
			Result: deterministic,
			Source: SourceDeterministic,
		}, nil
	}

	if provider == nil {
		return Resolution{
			Result: deterministic,
			Source: SourceDeterministic,
		}, nil
	}

	request := modelprovider.ClassificationRequest{
		Task:                task,
		ProjectContext:      projectContext,
		AllowedIntents:      allowedIntents(),
		AllowedCapabilities: activeCapabilityIDs(capabilities),
	}

	response, err := provider.Classify(ctx, request)
	if err != nil {
		return Resolution{}, fmt.Errorf("AI classification via %s: %w", provider.ID(), err)
	}

	if !response.CanClassify {
		question := strings.TrimSpace(response.ClarificationQuestion)
		if question == "" {
			question = defaultClarificationQuestion(activeCapabilityIDs(capabilities))
		}

		return Resolution{
			Result:                deterministic,
			Source:                SourceAI,
			Provider:              provider.ID(),
			ClarificationRequired: true,
			ClarificationQuestion: question,
		}, nil
	}

	intent, resolvedCapabilities, err := validateAIClassification(response, capabilities)
	if err != nil {
		return Resolution{}, fmt.Errorf("invalid AI classification from %s: %w", provider.ID(), err)
	}

	reasons := []string{
		"ai-provider:" + provider.ID(),
		"ai-intent:" + string(intent),
	}
	for _, capability := range resolvedCapabilities {
		reasons = append(reasons, "ai-capability:"+capability)
	}
	if rationale := strings.TrimSpace(response.Rationale); rationale != "" {
		reasons = append(reasons, "ai-rationale:"+rationale)
	}
	sort.Strings(reasons)

	return Resolution{
		Result: Result{
			Task:         strings.TrimSpace(task),
			Intent:       intent,
			Capabilities: resolvedCapabilities,
			Confidence:   ConfidenceHigh,
			NeedsAI:      false,
			Reasons:      reasons,
		},
		Source:   SourceAI,
		Provider: provider.ID(),
	}, nil
}

func validateAIClassification(
	response modelprovider.ClassificationResponse,
	capabilities registry.CapabilityRegistry,
) (orchestration.Intent, []string, error) {
	if !validAIIntent(response.Intent) {
		return "", nil, fmt.Errorf("unsupported intent %q", response.Intent)
	}

	if len(response.Capabilities) == 0 {
		return "", nil, fmt.Errorf("no capabilities returned")
	}

	var resolved []string
	for _, capabilityID := range response.Capabilities {
		capabilityID = strings.TrimSpace(capabilityID)
		capability, ok := capabilities.Capabilities[capabilityID]
		if !ok {
			return "", nil, fmt.Errorf("unknown capability %q", capabilityID)
		}
		if capability.Status != "active" {
			return "", nil, fmt.Errorf("capability %q is not active", capabilityID)
		}
		resolved = append(resolved, capabilityID)
	}

	return response.Intent, uniqueSorted(resolved), nil
}

func activeCapabilityIDs(registry registry.CapabilityRegistry) []string {
	var ids []string
	for id, capability := range registry.Capabilities {
		if capability.Status == "active" {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

func allowedIntents() []orchestration.Intent {
	return []orchestration.Intent{
		orchestration.IntentImplement,
		orchestration.IntentDesign,
		orchestration.IntentResearch,
		orchestration.IntentReview,
		orchestration.IntentVerify,
	}
}

func validAIIntent(intent orchestration.Intent) bool {
	for _, allowed := range allowedIntents() {
		if intent == allowed {
			return true
		}
	}
	return false
}

func defaultClarificationQuestion(activeCapabilities []string) string {
	if len(activeCapabilities) == 0 {
		return "¿Qué querés lograr exactamente con esta tarea?"
	}
	return "¿Qué querés lograr exactamente y sobre qué parte del sistema querés trabajar?"
}
