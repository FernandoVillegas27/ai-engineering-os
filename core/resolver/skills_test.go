package resolver

import (
	"testing"

	"github.com/FernandoVillegas27/ai-engineering-os/core/detector"
	"github.com/FernandoVillegas27/ai-engineering-os/core/registry"
)

func TestResolveSkillsSeparatesCandidatesFromEnabled(t *testing.T) {
	detection := detector.Detection{
		Backend: []detector.Technology{
			{Name: "java", Version: "21", Evidence: []string{"pom.xml"}},
			{Name: "spring-boot", Version: "3", Evidence: []string{"pom.xml"}},
		},
	}

	skills := registry.SkillRegistry{
		SchemaVersion: 1,
		Skills: map[string]registry.Skill{
			"java-21": {
				ID:           "java-21",
				Status:       "candidate",
				Trust:        registry.SkillTrust{Level: "community", Reviewed: false},
				Technologies: []registry.Technology{{Name: "java", Versions: []string{"21"}}},
				CompatibleAgents: []string{"backend-engineer"},
			},
			"spring-boot-3": {
				ID:           "spring-boot-3",
				Status:       "enabled",
				Trust:        registry.SkillTrust{Level: "company", Reviewed: true},
				Technologies: []registry.Technology{{Name: "spring-boot", Versions: []string{"3"}}},
				CompatibleAgents: []string{"backend-engineer"},
			},
		},
	}

	got := ResolveSkills(detection, skills, SkillOptions{})

	if len(got.Enabled) != 1 || got.Enabled[0].SkillID != "spring-boot-3" {
		t.Fatalf("enabled = %#v", got.Enabled)
	}
	if len(got.Candidates) != 1 || got.Candidates[0].SkillID != "java-21" {
		t.Fatalf("candidates = %#v", got.Candidates)
	}
}

func TestResolveSkillsHonorsAgentCompatibility(t *testing.T) {
	detection := detector.Detection{
		Frontend: []detector.Technology{{Name: "angular", Version: "20"}},
	}

	skills := registry.SkillRegistry{
		SchemaVersion: 1,
		Skills: map[string]registry.Skill{
			"angular-core": {
				ID:               "angular-core",
				Status:           "candidate",
				Trust:            registry.SkillTrust{Level: "curated"},
				Technologies:     []registry.Technology{{Name: "angular"}},
				CompatibleAgents: []string{"frontend-engineer"},
			},
		},
	}

	got := ResolveSkills(detection, skills, SkillOptions{AgentID: "backend-engineer"})
	if len(got.Candidates) != 0 {
		t.Fatalf("expected no candidate skills, got %#v", got.Candidates)
	}
}

func TestResolveSkillsHonorsVersionCompatibility(t *testing.T) {
	detection := detector.Detection{
		Backend: []detector.Technology{{Name: "java", Version: "17"}},
	}

	skills := registry.SkillRegistry{
		SchemaVersion: 1,
		Skills: map[string]registry.Skill{
			"java-21": {
				ID:               "java-21",
				Status:           "candidate",
				Trust:            registry.SkillTrust{Level: "community"},
				Technologies:     []registry.Technology{{Name: "java", Versions: []string{"21"}}},
				CompatibleAgents: []string{"backend-engineer"},
			},
		},
	}

	got := ResolveSkills(detection, skills, SkillOptions{})
	if len(got.Candidates) != 0 {
		t.Fatalf("expected java-21 not to match java 17, got %#v", got.Candidates)
	}
	if len(got.UnmatchedTechnologies) != 1 || got.UnmatchedTechnologies[0].Name != "java" {
		t.Fatalf("unmatched = %#v", got.UnmatchedTechnologies)
	}
}
