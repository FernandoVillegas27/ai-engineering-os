package skillcontent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/FernandoVillegas27/ai-engineering-os/core/registry"
	"gopkg.in/yaml.v3"
)

type Content struct {
	SkillID      string
	Name         string
	Description  string
	Author       string
	Version      string
	SourceType   string
	Provider     string
	Repository   string
	Revision     string
	Path         string
	Body         string
	Raw          string
}

type Options struct {
	BaseDir          string
	AllowCandidate   bool
	HTTPClient       *http.Client
	GitHubRawBaseURL string
}

type frontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Metadata    struct {
		Author  string `yaml:"author"`
		Version string `yaml:"version"`
	} `yaml:"metadata"`
}

func Load(ctx context.Context, skill registry.Skill, options Options) (Content, error) {
	if skill.ID == "" {
		return Content{}, errors.New("skill id is required")
	}

	if skill.Status == "disabled" || skill.Status == "deprecated" {
		return Content{}, fmt.Errorf("skill %q cannot be loaded because status is %q", skill.ID, skill.Status)
	}

	if skill.Status == "candidate" && !options.AllowCandidate {
		return Content{}, fmt.Errorf("skill %q is a candidate and requires explicit approval to load", skill.ID)
	}

	var (
		raw      []byte
		revision string
		err      error
	)

	switch skill.Source.Type {
	case "company", "generated":
		raw, err = loadLocal(skill, options)
	case "external":
		raw, revision, err = loadExternalGitHub(ctx, skill, options)
	default:
		return Content{}, fmt.Errorf("unsupported source type %q for skill %q", skill.Source.Type, skill.ID)
	}
	if err != nil {
		return Content{}, err
	}

	meta, body, err := parseSkillDocument(string(raw))
	if err != nil {
		return Content{}, fmt.Errorf("parse skill %q: %w", skill.ID, err)
	}

	if meta.Name == "" {
		return Content{}, fmt.Errorf("skill %q content has no frontmatter name", skill.ID)
	}
	if meta.Name != skill.ID {
		return Content{}, fmt.Errorf("skill registry id %q does not match content name %q", skill.ID, meta.Name)
	}

	repository := ""
	if skill.Source.Repository != nil {
		repository = *skill.Source.Repository
	}

	return Content{
		SkillID:     skill.ID,
		Name:        meta.Name,
		Description: meta.Description,
		Author:      meta.Metadata.Author,
		Version:     meta.Metadata.Version,
		SourceType:  skill.Source.Type,
		Provider:    skill.Source.Provider,
		Repository:  repository,
		Revision:    revision,
		Path:        skill.Source.Path,
		Body:        body,
		Raw:         string(raw),
	}, nil
}

func loadLocal(skill registry.Skill, options Options) ([]byte, error) {
	if strings.TrimSpace(options.BaseDir) == "" {
		return nil, fmt.Errorf("base directory is required to load local skill %q", skill.ID)
	}

	base, err := filepath.Abs(options.BaseDir)
	if err != nil {
		return nil, fmt.Errorf("resolve base directory: %w", err)
	}

	target, err := safeJoin(base, skill.Source.Path)
	if err != nil {
		return nil, fmt.Errorf("resolve local skill %q: %w", skill.ID, err)
	}

	data, err := os.ReadFile(target)
	if err != nil {
		return nil, fmt.Errorf("read local skill %q: %w", skill.ID, err)
	}
	return data, nil
}

func loadExternalGitHub(ctx context.Context, skill registry.Skill, options Options) ([]byte, string, error) {
	if skill.Source.Repository == nil || strings.TrimSpace(*skill.Source.Repository) == "" {
		return nil, "", fmt.Errorf("external skill %q has no repository", skill.ID)
	}
	if skill.Source.Version == nil || strings.TrimSpace(*skill.Source.Version) == "" {
		return nil, "", fmt.Errorf("external skill %q has no pinned version", skill.ID)
	}

	revision, err := parseGitRevision(*skill.Source.Version)
	if err != nil {
		return nil, "", fmt.Errorf("external skill %q: %w", skill.ID, err)
	}

	repository := strings.TrimSpace(*skill.Source.Repository)
	parts := strings.Split(repository, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, "", fmt.Errorf("external skill %q repository must use owner/name format", skill.ID)
	}

	path := strings.TrimPrefix(filepath.ToSlash(skill.Source.Path), "/")
	if path == "" || strings.Contains(path, "..") {
		return nil, "", fmt.Errorf("external skill %q has an unsafe path", skill.ID)
	}

	baseURL := strings.TrimRight(options.GitHubRawBaseURL, "/")
	if baseURL == "" {
		baseURL = "https://raw.githubusercontent.com"
	}

	url := fmt.Sprintf("%s/%s/%s/%s", baseURL, repository, revision, path)

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", fmt.Errorf("create request for skill %q: %w", skill.ID, err)
	}

	client := options.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	response, err := client.Do(request)
	if err != nil {
		return nil, "", fmt.Errorf("fetch external skill %q: %w", skill.ID, err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("fetch external skill %q: unexpected HTTP status %d", skill.ID, response.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return nil, "", fmt.Errorf("read external skill %q: %w", skill.ID, err)
	}
	if len(body) == 0 {
		return nil, "", fmt.Errorf("external skill %q returned empty content", skill.ID)
	}

	return body, revision, nil
}

func parseSkillDocument(raw string) (frontmatter, string, error) {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	if !strings.HasPrefix(raw, "---\n") {
		return frontmatter{}, "", errors.New("missing YAML frontmatter")
	}

	rest := strings.TrimPrefix(raw, "---\n")
	index := strings.Index(rest, "\n---\n")
	if index < 0 {
		return frontmatter{}, "", errors.New("unterminated YAML frontmatter")
	}

	frontmatterRaw := rest[:index]
	body := rest[index+5:]

	var meta frontmatter
	if err := yaml.Unmarshal([]byte(frontmatterRaw), &meta); err != nil {
		return frontmatter{}, "", fmt.Errorf("invalid YAML frontmatter: %w", err)
	}

	return meta, body, nil
}

func parseGitRevision(version string) (string, error) {
	version = strings.TrimSpace(version)
	if !strings.HasPrefix(version, "git:") {
		return "", fmt.Errorf("version %q must be pinned using git:<revision>", version)
	}
	revision := strings.TrimPrefix(version, "git:")
	if revision == "" {
		return "", errors.New("git revision is empty")
	}
	for _, r := range revision {
		if !((r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F') || (r >= '0' && r <= '9')) {
			return "", fmt.Errorf("git revision %q is not a hexadecimal commit id", revision)
		}
	}
	if len(revision) < 7 || len(revision) > 40 {
		return "", fmt.Errorf("git revision %q must contain between 7 and 40 hexadecimal characters", revision)
	}
	return revision, nil
}

func safeJoin(base, relative string) (string, error) {
	relative = filepath.Clean(relative)
	if filepath.IsAbs(relative) {
		return "", errors.New("absolute paths are not allowed")
	}

	target := filepath.Join(base, relative)
	relativeToBase, err := filepath.Rel(base, target)
	if err != nil {
		return "", err
	}
	if relativeToBase == ".." || strings.HasPrefix(relativeToBase, ".."+string(filepath.Separator)) {
		return "", errors.New("path escapes the configured base directory")
	}
	return target, nil
}
