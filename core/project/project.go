package project

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/FernandoVillegas27/ai-engineering-os/core/detector"
	"github.com/FernandoVillegas27/ai-engineering-os/core/registry"
	"github.com/FernandoVillegas27/ai-engineering-os/core/resolver"
	"gopkg.in/yaml.v3"
)

type File struct {
	Project Project `yaml:"project"`
}

type Project struct {
	ID           string            `yaml:"id"`
	Name         string            `yaml:"name"`
	Repository   Repository        `yaml:"repository"`
	Detected     detector.Detection `yaml:"detected"`
	Architecture Architecture      `yaml:"architecture"`
	Skills       Skills            `yaml:"skills"`
	Memory       Memory            `yaml:"memory"`
}

type Repository struct {
	Type          string  `yaml:"type"`
	Remote        *string `yaml:"remote"`
	DefaultBranch *string `yaml:"default_branch"`
}

type Architecture struct {
	Style string  `yaml:"style"`
	Notes *string `yaml:"notes"`
}

type Skills struct {
	Recommended []string `yaml:"recommended"`
	Candidates  []string `yaml:"candidates"`
	Missing     []string `yaml:"missing"`
}

type Memory struct {
	Provider  string `yaml:"provider"`
	Namespace string `yaml:"namespace"`
}

func Build(root, skillRegistryPath string) (File, error) {
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return File{}, fmt.Errorf("resolve project root: %w", err)
	}

	detection, err := detector.Inspect(absoluteRoot)
	if err != nil {
		return File{}, fmt.Errorf("inspect project: %w", err)
	}

	skillRegistry, err := registry.LoadSkills(skillRegistryPath)
	if err != nil {
		return File{}, err
	}

	resolution := resolver.ResolveSkills(detection, skillRegistry, resolver.SkillOptions{})

	name := filepath.Base(filepath.Clean(absoluteRoot))
	id := slugify(name)
	if id == "" {
		return File{}, fmt.Errorf("project name %q cannot be converted to a valid id", name)
	}

	return File{
		Project: Project{
			ID:         id,
			Name:       name,
			Repository: inspectRepository(absoluteRoot),
			Detected:   detection,
			Architecture: Architecture{
				Style: "unknown",
				Notes: nil,
			},
			Skills: Skills{
				Recommended: matchIDs(resolution.Enabled),
				Candidates:  matchIDs(resolution.Candidates),
				Missing:     unmatchedNames(resolution.UnmatchedTechnologies),
			},
			Memory: Memory{
				Provider:  "engram",
				Namespace: id,
			},
		},
	}, nil
}

func Marshal(file File) ([]byte, error) {
	data, err := yaml.Marshal(file)
	if err != nil {
		return nil, fmt.Errorf("marshal project context: %w", err)
	}
	return data, nil
}

func Write(root string, file File) (string, error) {
	data, err := Marshal(file)
	if err != nil {
		return "", err
	}

	outputDir := filepath.Join(root, ".ai")
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", fmt.Errorf("create .ai directory: %w", err)
	}

	outputPath := filepath.Join(outputDir, "project.yaml")
	if err := os.WriteFile(outputPath, data, 0o644); err != nil {
		return "", fmt.Errorf("write project context: %w", err)
	}

	return outputPath, nil
}

func inspectRepository(root string) Repository {
	repository := Repository{Type: "none"}

	if _, err := runGit(root, "rev-parse", "--is-inside-work-tree"); err != nil {
		return repository
	}

	repository.Type = "git"

	if remote, err := runGit(root, "config", "--get", "remote.origin.url"); err == nil && remote != "" {
		repository.Remote = stringPointer(remote)
	}

	if branch, err := runGit(root, "symbolic-ref", "--short", "refs/remotes/origin/HEAD"); err == nil {
		branch = strings.TrimPrefix(branch, "origin/")
		if branch != "" {
			repository.DefaultBranch = stringPointer(branch)
		}
	}

	return repository
}

func runGit(root string, args ...string) (string, error) {
	commandArgs := append([]string{"-C", root}, args...)
	output, err := exec.Command("git", commandArgs...).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func matchIDs(matches []resolver.SkillMatch) []string {
	ids := make([]string, 0, len(matches))
	for _, match := range matches {
		ids = append(ids, match.SkillID)
	}
	sort.Strings(ids)
	return ids
}

func unmatchedNames(technologies []detector.Technology) []string {
	seen := map[string]bool{}
	names := make([]string, 0, len(technologies))
	for _, technology := range technologies {
		if seen[technology.Name] {
			continue
		}
		seen[technology.Name] = true
		names = append(names, technology.Name)
	}
	sort.Strings(names)
	return names
}

func slugify(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	nonAlphaNumeric := regexp.MustCompile(`[^a-z0-9]+`)
	value = nonAlphaNumeric.ReplaceAllString(value, "-")
	return strings.Trim(value, "-")
}

func stringPointer(value string) *string {
	return &value
}
