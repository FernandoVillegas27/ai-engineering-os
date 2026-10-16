package registry

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type SkillRegistry struct {
	SchemaVersion int              `yaml:"schema_version"`
	Skills        map[string]Skill `yaml:"skills"`
}

type Skill struct {
	ID               string       `yaml:"id"`
	Description      string       `yaml:"description"`
	Source           SkillSource  `yaml:"source"`
	Trust            SkillTrust   `yaml:"trust"`
	Capabilities     []string     `yaml:"capabilities"`
	Domains          []string     `yaml:"domains"`
	Technologies     []Technology `yaml:"technologies"`
	CompatibleAgents []string     `yaml:"compatible_agents"`
	Tools            SkillTools   `yaml:"tools"`
	Activation       Activation   `yaml:"activation"`
	Status           string       `yaml:"status"`
}

type SkillSource struct {
	Type       string  `yaml:"type"`
	Provider   string  `yaml:"provider"`
	Repository *string `yaml:"repository"`
	Path       string  `yaml:"path"`
	Version    *string `yaml:"version"`
}

type SkillTrust struct {
	Level    string `yaml:"level"`
	Reviewed bool   `yaml:"reviewed"`
}

type Technology struct {
	Name     string   `yaml:"name"`
	Versions []string `yaml:"versions"`
}

type SkillTools struct {
	Required    []string `yaml:"required"`
	Recommended []string `yaml:"recommended"`
}

type Activation struct {
	Triggers       []string `yaml:"triggers"`
	ProjectSignals []string `yaml:"project_signals"`
}

func LoadSkills(path string) (SkillRegistry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return SkillRegistry{}, fmt.Errorf("read skill registry: %w", err)
	}

	var registry SkillRegistry
	if err := yaml.Unmarshal(data, &registry); err != nil {
		return SkillRegistry{}, fmt.Errorf("parse skill registry: %w", err)
	}

	if registry.SchemaVersion <= 0 {
		return SkillRegistry{}, fmt.Errorf("skill registry schema_version must be greater than zero")
	}
	if registry.Skills == nil {
		registry.Skills = map[string]Skill{}
	}

	for key, skill := range registry.Skills {
		if skill.ID == "" {
			return SkillRegistry{}, fmt.Errorf("skill %q has no id", key)
		}
		if key != skill.ID {
			return SkillRegistry{}, fmt.Errorf("skill key %q does not match id %q", key, skill.ID)
		}
	}

	return registry, nil
}
