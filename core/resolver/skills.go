package resolver

import (
	"sort"

	"github.com/FernandoVillegas27/ai-engineering-os/core/detector"
	"github.com/FernandoVillegas27/ai-engineering-os/core/registry"
)

type SkillMatch struct {
	SkillID      string
	Status       string
	TrustLevel   string
	Reviewed     bool
	Technologies []string
	Reasons      []string
}

type SkillResolution struct {
	Enabled                []SkillMatch
	Candidates             []SkillMatch
	UnmatchedTechnologies  []detector.Technology
}

type SkillOptions struct {
	AgentID     string
	Capabilities []string
}

func ResolveSkills(detection detector.Detection, skills registry.SkillRegistry, options SkillOptions) SkillResolution {
	detected := uniqueTechnologies(detection)
	matchedTechnology := map[string]bool{}

	var result SkillResolution

	ids := make([]string, 0, len(skills.Skills))
	for id := range skills.Skills {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	for _, id := range ids {
		skill := skills.Skills[id]

		if skill.Status == "disabled" || skill.Status == "deprecated" {
			continue
		}
		if options.AgentID != "" && !contains(skill.CompatibleAgents, options.AgentID) {
			continue
		}
		if len(options.Capabilities) > 0 && !intersects(skill.Capabilities, options.Capabilities) {
			continue
		}

		match := matchSkill(skill, detected)
		if len(match.Technologies) == 0 {
			continue
		}

		for _, name := range match.Technologies {
			matchedTechnology[name] = true
		}

		switch skill.Status {
		case "enabled":
			result.Enabled = append(result.Enabled, match)
		case "candidate":
			result.Candidates = append(result.Candidates, match)
		}
	}

	for _, technology := range detected {
		if !matchedTechnology[technology.Name] {
			result.UnmatchedTechnologies = append(result.UnmatchedTechnologies, technology)
		}
	}

	return result
}

func matchSkill(skill registry.Skill, detected []detector.Technology) SkillMatch {
	match := SkillMatch{
		SkillID:    skill.ID,
		Status:     skill.Status,
		TrustLevel: skill.Trust.Level,
		Reviewed:   skill.Trust.Reviewed,
	}

	for _, requirement := range skill.Technologies {
		for _, technology := range detected {
			if requirement.Name != technology.Name {
				continue
			}
			if !versionMatches(technology.Version, requirement.Versions) {
				continue
			}

			match.Technologies = appendUnique(match.Technologies, technology.Name)

			reason := "technology:" + technology.Name
			if technology.Version != "" {
				reason += "@" + technology.Version
			}
			match.Reasons = appendUnique(match.Reasons, reason)
		}
	}

	sort.Strings(match.Technologies)
	sort.Strings(match.Reasons)

	return match
}

func uniqueTechnologies(detection detector.Detection) []detector.Technology {
	all := [][]detector.Technology{
		detection.Frontend,
		detection.Backend,
		detection.Database,
		detection.Testing,
		detection.Build,
		detection.Runtime,
		detection.Infrastructure,
	}

	byKey := map[string]detector.Technology{}
	for _, group := range all {
		for _, technology := range group {
			key := technology.Name + "@" + technology.Version
			current, exists := byKey[key]
			if !exists {
				byKey[key] = technology
				continue
			}
			current.Evidence = mergeStrings(current.Evidence, technology.Evidence)
			byKey[key] = current
		}
	}

	out := make([]detector.Technology, 0, len(byKey))
	for _, technology := range byKey {
		out = append(out, technology)
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Name == out[j].Name {
			return out[i].Version < out[j].Version
		}
		return out[i].Name < out[j].Name
	})

	return out
}

func versionMatches(detected string, supported []string) bool {
	if len(supported) == 0 {
		return true
	}
	if detected == "" {
		return false
	}

	for _, version := range supported {
		if detected == version {
			return true
		}
	}
	return false
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func intersects(a, b []string) bool {
	for _, left := range a {
		if contains(b, left) {
			return true
		}
	}
	return false
}

func appendUnique(values []string, value string) []string {
	if contains(values, value) {
		return values
	}
	return append(values, value)
}

func mergeStrings(a, b []string) []string {
	out := append([]string{}, a...)
	for _, value := range b {
		out = appendUnique(out, value)
	}
	sort.Strings(out)
	return out
}
