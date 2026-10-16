package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildProjectContextSeparatesCandidateSkills(t *testing.T) {
	root := t.TempDir()
	registryPath := filepath.Join(t.TempDir(), "skills.yaml")

	writeFile(t, filepath.Join(root, "pom.xml"), `<project>
		<parent>
			<artifactId>spring-boot-starter-parent</artifactId>
			<version>3.5.0</version>
		</parent>
		<properties>
			<java.version>21</java.version>
		</properties>
	</project>`)

	writeFile(t, registryPath, `schema_version: 1
skills:
  java-21:
    id: java-21
    description: Java 21 engineering guidance for backend applications.
    source:
      type: external
      provider: test
      path: java-21/SKILL.md
    trust:
      level: community
      reviewed: false
    capabilities:
      - backend-development
    domains:
      - backend
    technologies:
      - name: java
        versions:
          - "21"
    compatible_agents:
      - backend-engineer
    status: candidate

  spring-boot-3:
    id: spring-boot-3
    description: Spring Boot 3 engineering guidance for backend APIs.
    source:
      type: company
      provider: test
      path: spring-boot-3/SKILL.md
    trust:
      level: company
      reviewed: true
    capabilities:
      - backend-development
    domains:
      - backend
    technologies:
      - name: spring-boot
        versions:
          - "3"
    compatible_agents:
      - backend-engineer
    status: enabled
`)

	got, err := Build(root, registryPath)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if len(got.Project.Skills.Recommended) != 1 || got.Project.Skills.Recommended[0] != "spring-boot-3" {
		t.Fatalf("recommended = %#v", got.Project.Skills.Recommended)
	}
	if len(got.Project.Skills.Candidates) != 1 || got.Project.Skills.Candidates[0] != "java-21" {
		t.Fatalf("candidates = %#v", got.Project.Skills.Candidates)
	}
	if got.Project.Architecture.Style != "unknown" {
		t.Fatalf("architecture style = %q", got.Project.Architecture.Style)
	}
	if got.Project.Memory.Provider != "engram" {
		t.Fatalf("memory provider = %q", got.Project.Memory.Provider)
	}
}

func TestWriteCreatesAIProjectFile(t *testing.T) {
	root := t.TempDir()
	file := File{
		Project: Project{
			ID:   "demo",
			Name: "demo",
			Repository: Repository{Type: "none"},
			Architecture: Architecture{Style: "unknown"},
			Skills: Skills{},
			Memory: Memory{Provider: "engram", Namespace: "demo"},
		},
	}

	path, err := Write(root, file)
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	if path != filepath.Join(root, ".ai", "project.yaml") {
		t.Fatalf("path = %q", path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	if !strings.Contains(string(data), "project:") || !strings.Contains(string(data), "namespace: demo") {
		t.Fatalf("unexpected output:\n%s", data)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
