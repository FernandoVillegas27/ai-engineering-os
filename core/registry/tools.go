package registry

import (
	"fmt"
	"os"
	"strings"

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
	RequiresApproval            bool     `yaml:"requires_approval"`
	ApprovalRequiredOperations []string `yaml:"approval_required_operations,omitempty"`
	AvailabilityCheck          *string      `yaml:"availability_check"`
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
		if err := validateTool(tool); err != nil {
			return ToolRegistry{}, fmt.Errorf("tool %q: %w", key, err)
		}
	}
	return registry, nil
}

func validateTool(tool Tool) error {
	if strings.TrimSpace(tool.Description) == "" {
		return fmt.Errorf("description is required")
	}
	switch strings.TrimSpace(tool.Interface.Type) {
	case "native":
		// Native tools may omit provider when the operation is implemented by core.
	case "mcp", "connector":
		if tool.Interface.Provider == nil || strings.TrimSpace(*tool.Interface.Provider) == "" {
			return fmt.Errorf("%s interface requires provider", tool.Interface.Type)
		}
		if tool.AvailabilityCheck == nil || strings.TrimSpace(*tool.AvailabilityCheck) == "" {
			return fmt.Errorf("%s interface requires availability_check", tool.Interface.Type)
		}
	default:
		return fmt.Errorf("unsupported interface type %q", tool.Interface.Type)
	}
	if len(tool.Operations) == 0 {
		return fmt.Errorf("at least one operation is required")
	}
	for _, operation := range tool.Operations {
		if strings.TrimSpace(operation) == "" {
			return fmt.Errorf("operations must not contain empty values")
		}
	}
	if len(tool.ApprovalRequiredOperations) > 0 {
		declared := map[string]bool{}
		for _, operation := range tool.Operations {
			declared[operation] = true
		}
		seenApproval := map[string]bool{}
		for _, operation := range tool.ApprovalRequiredOperations {
			operation = strings.TrimSpace(operation)
			if operation == "" {
				return fmt.Errorf("approval_required_operations must not contain empty values")
			}
			if seenApproval[operation] {
				return fmt.Errorf("duplicate approval-required operation %q", operation)
			}
			seenApproval[operation] = true
			if !declared[operation] {
				return fmt.Errorf(
					"approval-required operation %q is not declared in operations",
					operation,
				)
			}
		}
	}
	if len(tool.Access) == 0 {
		return fmt.Errorf("at least one access mode is required")
	}
	for _, access := range tool.Access {
		switch strings.TrimSpace(access) {
		case "read", "write", "execute", "network":
		default:
			return fmt.Errorf("unsupported access mode %q", access)
		}
	}
	switch strings.TrimSpace(tool.Risk) {
	case "low", "medium", "high":
	default:
		return fmt.Errorf("risk must be low, medium, or high")
	}
	switch strings.TrimSpace(tool.Status) {
	case "enabled", "disabled":
	default:
		return fmt.Errorf("status must be enabled or disabled")
	}
	return nil
}


func OperationRequiresApproval(tool Tool, operation string) bool {
	operation = strings.TrimSpace(operation)
	if operation == "" {
		return false
	}
	if len(tool.ApprovalRequiredOperations) > 0 {
		for _, candidate := range tool.ApprovalRequiredOperations {
			if strings.TrimSpace(candidate) == operation {
				return true
			}
		}
		return false
	}
	return tool.RequiresApproval
}
