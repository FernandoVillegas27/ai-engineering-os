package orchestration

import (
	"reflect"
	"testing"

	"github.com/FernandoVillegas27/ai-engineering-os/core/detector"
	"github.com/FernandoVillegas27/ai-engineering-os/core/registry"
)

func TestBuildSelectsSingleAgentForBackendAPI(t *testing.T) {
	agents := registry.AgentRegistry{Agents: map[string]registry.Agent{
		"backend-engineer": {
			ID:           "backend-engineer",
			Role:         "backend-engineer",
			Capabilities: []string{"backend-development", "api-development", "automated-testing"},
			Tools:        registry.AgentTools{Required: []string{"filesystem"}, Optional: []string{"context7", "engram"}},
			Status:       "active",
		},
		"architect": {
			ID:           "architect",
			Role:         "architect",
			Capabilities: []string{"api-development"},
			Status:       "active",
		},
	}}
	skills := registry.SkillRegistry{SchemaVersion: 1, Skills: map[string]registry.Skill{
		"spring-boot-3": {
			ID:               "spring-boot-3",
			Capabilities:     []string{"backend-development", "api-development"},
			Technologies:     []registry.Technology{{Name: "spring-boot", Versions: []string{"3"}}},
			CompatibleAgents: []string{"backend-engineer"},
			Tools:            registry.SkillTools{Recommended: []string{"context7"}},
			Status:           "candidate",
		},
	}}
	tools := registry.ToolRegistry{SchemaVersion: 1, Tools: map[string]registry.Tool{
		"filesystem": {ID: "filesystem", Status: "enabled"},
		"context7":   {ID: "context7", Status: "enabled"},
		"engram":     {ID: "engram", Status: "enabled"},
	}}
	detection := detector.Detection{
		Backend: []detector.Technology{{Name: "spring-boot", Version: "3"}},
	}

	plan, err := Build(Request{Capabilities: []string{"api-development", "backend-development"}}, detection, agents, skills, tools)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(plan.Agents) != 1 || plan.Agents[0].AgentID != "backend-engineer" {
		t.Fatalf("agents = %#v", plan.Agents)
	}
	if !reflect.DeepEqual(plan.Agents[0].Capabilities, []string{"api-development", "backend-development"}) {
		t.Fatalf("capabilities = %#v", plan.Agents[0].Capabilities)
	}
	if !reflect.DeepEqual(plan.Agents[0].Skills.Candidates, []string{"spring-boot-3"}) {
		t.Fatalf("candidate skills = %#v", plan.Agents[0].Skills.Candidates)
	}
}

func TestBuildUsesMultipleAgentsWhenNeeded(t *testing.T) {
	agents := registry.AgentRegistry{Agents: map[string]registry.Agent{
		"frontend-engineer": {
			ID: "frontend-engineer", Role: "frontend-engineer",
			Capabilities: []string{"frontend-development"}, Status: "active",
		},
		"database-engineer": {
			ID: "database-engineer", Role: "database-engineer",
			Capabilities: []string{"database-design"}, Status: "active",
		},
	}}
	plan, err := Build(
		Request{Capabilities: []string{"frontend-development", "database-design"}},
		detector.Detection{},
		agents,
		registry.SkillRegistry{SchemaVersion: 1, Skills: map[string]registry.Skill{}},
		registry.ToolRegistry{SchemaVersion: 1, Tools: map[string]registry.Tool{}},
	)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(plan.Agents) != 2 {
		t.Fatalf("agents = %#v", plan.Agents)
	}
}

func TestBuildReportsUncoveredCapabilities(t *testing.T) {
	plan, err := Build(
		Request{Capabilities: []string{"deployment"}},
		detector.Detection{},
		registry.AgentRegistry{Agents: map[string]registry.Agent{}},
		registry.SkillRegistry{SchemaVersion: 1, Skills: map[string]registry.Skill{}},
		registry.ToolRegistry{SchemaVersion: 1, Tools: map[string]registry.Tool{}},
	)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if !reflect.DeepEqual(plan.UncoveredCapabilities, []string{"deployment"}) {
		t.Fatalf("uncovered = %#v", plan.UncoveredCapabilities)
	}
}

func TestBuildExcludesOrchestratorAsWorker(t *testing.T) {
	agents := registry.AgentRegistry{Agents: map[string]registry.Agent{
		"orchestrator": {
			ID: "orchestrator", Role: "orchestrator",
			Capabilities: []string{"technical-research"}, Status: "active",
		},
		"researcher": {
			ID: "researcher", Role: "researcher",
			Capabilities: []string{"technical-research"}, Status: "active",
		},
	}}
	plan, err := Build(
		Request{Capabilities: []string{"technical-research"}},
		detector.Detection{},
		agents,
		registry.SkillRegistry{SchemaVersion: 1, Skills: map[string]registry.Skill{}},
		registry.ToolRegistry{SchemaVersion: 1, Tools: map[string]registry.Tool{}},
	)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(plan.Agents) != 1 || plan.Agents[0].AgentID != "researcher" {
		t.Fatalf("agents = %#v", plan.Agents)
	}
}
