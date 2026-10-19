package registry

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type CapabilityRegistry struct {
	Capabilities map[string]Capability `yaml:"capabilities"`
}

type Capability struct {
	Description string   `yaml:"description"`
	Category    string   `yaml:"category"`
	Aliases     []string `yaml:"aliases"`
	Status      string   `yaml:"status"`
}

func LoadCapabilities(path string) (CapabilityRegistry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return CapabilityRegistry{}, fmt.Errorf("read capability registry: %w", err)
	}

	var registry CapabilityRegistry
	if err := yaml.Unmarshal(data, &registry); err != nil {
		return CapabilityRegistry{}, fmt.Errorf("parse capability registry: %w", err)
	}
	if registry.Capabilities == nil {
		registry.Capabilities = map[string]Capability{}
	}

	for id, capability := range registry.Capabilities {
		if id == "" {
			return CapabilityRegistry{}, fmt.Errorf("capability id cannot be empty")
		}
		if capability.Status == "" {
			return CapabilityRegistry{}, fmt.Errorf("capability %q has no status", id)
		}
	}

	return registry, nil
}
