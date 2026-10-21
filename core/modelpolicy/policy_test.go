package modelpolicy

import "testing"

func TestResolveAutoStartsWithoutExplicitModel(t *testing.T) {
	got := Resolve(PolicyAuto, RoleProposal, "", false)
	if got.Model != "" || got.Escalated {
		t.Fatalf("decision=%#v", got)
	}
}

func TestResolveAutoEscalatesProposalToBalancedModel(t *testing.T) {
	got := Resolve(PolicyAuto, RoleProposal, "", true)
	if got.Model != BalancedEngineeringModel || !got.Escalated {
		t.Fatalf("decision=%#v", got)
	}
}

func TestResolveAutoEscalatedReviewUsesIndependentModel(t *testing.T) {
	got := Resolve(PolicyAuto, RoleReview, "", true)
	if got.Model != BalancedReviewModel {
		t.Fatalf("decision=%#v", got)
	}
}

func TestResolveEconomyNeverPinsModel(t *testing.T) {
	got := Resolve(PolicyEconomy, RoleProposal, "", true)
	if got.Model != "" {
		t.Fatalf("decision=%#v", got)
	}
}

func TestResolveBalancedAndDeep(t *testing.T) {
	if got := Resolve(PolicyBalanced, RoleStage, "", false).Model; got != BalancedEngineeringModel {
		t.Fatalf("balanced model=%q", got)
	}
	if got := Resolve(PolicyDeep, RoleStage, "", false).Model; got != DeepEngineeringModel {
		t.Fatalf("deep model=%q", got)
	}
	if got := Resolve(PolicyDeep, RoleReview, "", false).Model; got != DeepReviewModel {
		t.Fatalf("deep review model=%q", got)
	}
}

func TestResolveExplicitOverrideWins(t *testing.T) {
	got := Resolve(PolicyEconomy, RoleProposal, "custom-model", false)
	if got.Model != "custom-model" {
		t.Fatalf("decision=%#v", got)
	}
}

func TestParseRejectsUnknownPolicy(t *testing.T) {
	if _, err := Parse("turbo"); err == nil {
		t.Fatal("expected unsupported policy error")
	}
}
