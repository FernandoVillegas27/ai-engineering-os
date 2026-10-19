package registry

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type AgentRegistry struct {
	Agents map[string]Agent `yaml:"agents"`
}

type Agent struct {
	ID           string           `yaml:"id"`
	Role         string           `yaml:"role"`
	Mission      string           `yaml:"mission"`
	Capabilities []string         `yaml:"capabilities"`
	SkillDomains []string         `yaml:"skill_domains"`
	Tools        AgentTools       `yaml:"tools"`
	Permissions  AgentPermissions `yaml:"permissions"`
	Status       string           `yaml:"status"`
}

type AgentTools struct {
	Required []string `yaml:"required"`
	Optional []string `yaml:"optional"`
}

type AgentPermissions struct {
	SourceCode string `yaml:"source_code"`
	Tests      string `yaml:"tests"`
	GitCommit  string `yaml:"git_commit"`
	GitPush    string `yaml:"git_push"`
	Secrets    string `yaml:"secrets"`
	Production string `yaml:"production"`
}

func LoadAgents(path string) (AgentRegistry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return AgentRegistry{}, fmt.Errorf("read agent registry: %w", err)
	}

	var registry AgentRegistry
	if err := yaml.Unmarshal(data, &registry); err != nil {
		return AgentRegistry{}, fmt.Errorf("parse agent registry: %w", err)
	}
	if registry.Agents == nil {
		registry.Agents = map[string]Agent{}
	}

	for key, agent := range registry.Agents {
		if agent.ID == "" {
			return AgentRegistry{}, fmt.Errorf("agent %q has no id", key)
		}
		if key != agent.ID {
			return AgentRegistry{}, fmt.Errorf("agent key %q does not match id %q", key, agent.ID)
		}
	}

	return registry, nil
}
