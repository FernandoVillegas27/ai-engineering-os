package registry

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type ToolRegistry struct {
	SchemaVersion int             `yaml:"schema_version"`
	Tools         map[string]Tool `yaml:"tools"`
}

type Tool struct {
	ID               string        `yaml:"id"`
	Description      string        `yaml:"description"`
	Category         string        `yaml:"category"`
	Interface        ToolInterface `yaml:"interface"`
	Operations       []string      `yaml:"operations"`
	Access           []string      `yaml:"access"`
	Risk             string        `yaml:"risk"`
	RequiresApproval bool          `yaml:"requires_approval"`
	AvailabilityCheck *string      `yaml:"availability_check"`
	Status           string        `yaml:"status"`
}

type ToolInterface struct {
	Type     string  `yaml:"type"`
	Provider *string `yaml:"provider"`
}

func LoadTools(path string) (ToolRegistry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ToolRegistry{}, fmt.Errorf("read tool registry: %w", err)
	}

	var registry ToolRegistry
	if err := yaml.Unmarshal(data, &registry); err != nil {
		return ToolRegistry{}, fmt.Errorf("parse tool registry: %w", err)
	}
	if registry.SchemaVersion <= 0 {
		return ToolRegistry{}, fmt.Errorf("tool registry schema_version must be greater than zero")
	}
	if registry.Tools == nil {
		registry.Tools = map[string]Tool{}
	}
	for key, tool := range registry.Tools {
		if tool.ID == "" {
			return ToolRegistry{}, fmt.Errorf("tool %q has no id", key)
		}
		if key != tool.ID {
			return ToolRegistry{}, fmt.Errorf("tool key %q does not match id %q", key, tool.ID)
		}
	}
	return registry, nil
}
