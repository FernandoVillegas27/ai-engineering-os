package modelpolicy

import (
	"fmt"
	"strings"
)

type Policy string
type Role string

const (
	PolicyAuto     Policy = "auto"
	PolicyEconomy  Policy = "economy"
	PolicyBalanced Policy = "balanced"
	PolicyDeep     Policy = "deep"

	RoleProposal   Role = "proposal"
	RoleStage      Role = "stage"
	RoleReview     Role = "review"
	RoleCorrection Role = "correction"

	BalancedEngineeringModel = "gpt-5.6-sol-high"
	BalancedReviewModel      = "claude-sonnet-5-thinking-high"
	DeepEngineeringModel     = "gpt-5.6-sol-xhigh"
	DeepReviewModel          = "claude-opus-5-thinking-high"
)

type Decision struct {
	Policy    Policy
	Role      Role
	Model     string
	Escalated bool
	Reason    string
}

func Parse(value string) (Policy, error) {
	switch Policy(strings.ToLower(strings.TrimSpace(value))) {
	case PolicyAuto:
		return PolicyAuto, nil
	case PolicyEconomy:
		return PolicyEconomy, nil
	case PolicyBalanced:
		return PolicyBalanced, nil
	case PolicyDeep:
		return PolicyDeep, nil
	default:
		return "", fmt.Errorf("unsupported model policy %q; expected auto|economy|balanced|deep", value)
	}
}

func Resolve(policy Policy, role Role, override string, escalated bool) Decision {
	override = strings.TrimSpace(override)
	if override != "" {
		return Decision{
			Policy: policy, Role: role, Model: override, Escalated: escalated,
			Reason: "explicit model override",
		}
	}

	decision := Decision{Policy: policy, Role: role, Escalated: escalated}
	switch policy {
	case PolicyEconomy:
		decision.Reason = "economy policy keeps Cursor automatic model selection"
	case PolicyBalanced:
		decision.Model = balancedModel(role)
		decision.Reason = "balanced policy selects a fixed stronger model"
	case PolicyDeep:
		decision.Model = deepModel(role)
		decision.Reason = "deep policy selects a fixed high-reasoning model"
	case PolicyAuto:
		if escalated {
			decision.Model = balancedModel(role)
			decision.Reason = "auto policy escalated after deterministic evidence that automatic selection was insufficient"
		} else {
			decision.Reason = "auto policy starts with Cursor automatic model selection to conserve quota"
		}
	default:
		decision.Reason = "unknown policy falls back to Cursor automatic model selection"
	}
	return decision
}

func balancedModel(role Role) string {
	if role == RoleReview {
		return BalancedReviewModel
	}
	return BalancedEngineeringModel
}

func deepModel(role Role) string {
	if role == RoleReview {
		return DeepReviewModel
	}
	return DeepEngineeringModel
}
