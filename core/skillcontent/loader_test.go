package skillcontent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FernandoVillegas27/ai-engineering-os/core/registry"
)

func TestLoadExternalPinnedSkill(t *testing.T) {
	const revision = "c8036a37893679dc5e942484975405d39689c63b"
	const body = `---
name: spring-boot-3
description: >
  Spring Boot 3 patterns for backend applications.
metadata:
  author: gentleman-programming
  version: "1.0"
---

## When to Use

Use this for Spring Boot 3.
`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		want := "/Gentleman-Programming/Gentleman-Skills/" + revision + "/community/spring-boot-3/SKILL.md"
		if r.URL.Path != want {
			t.Fatalf("request path = %q, want %q", r.URL.Path, want)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	repository := "Gentleman-Programming/Gentleman-Skills"
	version := "git:" + revision
	skill := registry.Skill{
		ID:     "spring-boot-3",
		Status: "candidate",
		Source: registry.SkillSource{
			Type:       "external",
			Provider:   "gentleman-skills",
			Repository: &repository,
			Path:       "community/spring-boot-3/SKILL.md",
			Version:    &version,
		},
	}

	content, err := Load(context.Background(), skill, Options{
		AllowCandidate:   true,
		GitHubRawBaseURL: server.URL,
		HTTPClient:       server.Client(),
	})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if content.Name != "spring-boot-3" {
		t.Fatalf("name = %q", content.Name)
	}
	if content.Revision != revision {
		t.Fatalf("revision = %q", content.Revision)
	}
	if !strings.Contains(content.Body, "Spring Boot 3") {
		t.Fatalf("body = %q", content.Body)
	}
}

func TestLoadCandidateRequiresExplicitApproval(t *testing.T) {
	skill := registry.Skill{
		ID:     "typescript",
		Status: "candidate",
		Source: registry.SkillSource{Type: "external"},
	}

	_, err := Load(context.Background(), skill, Options{})
	if err == nil || !strings.Contains(err.Error(), "requires explicit approval") {
		t.Fatalf("expected candidate approval error, got %v", err)
	}
}

func TestLoadLocalCompanySkill(t *testing.T) {
	base := t.TempDir()
	path := filepath.Join(base, "skills", "company-api", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}

	raw := `---
name: company-api
description: >
  Internal API engineering conventions for company services.
metadata:
  author: engineering
  version: "1.0"
---

## Rules

Use explicit API contracts.
`
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}

	skill := registry.Skill{
		ID:     "company-api",
		Status: "enabled",
		Source: registry.SkillSource{
			Type:     "company",
			Provider: "ai-engineering-os",
			Path:     "skills/company-api/SKILL.md",
		},
	}

	content, err := Load(context.Background(), skill, Options{BaseDir: base})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if content.Author != "engineering" {
		t.Fatalf("author = %q", content.Author)
	}
}

func TestLoadRejectsContentNameMismatch(t *testing.T) {
	base := t.TempDir()
	path := filepath.Join(base, "skills", "expected", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`---
name: another-skill
description: >
  This is deliberately the wrong skill content.
---
body
`), 0o600); err != nil {
		t.Fatal(err)
	}

	skill := registry.Skill{
		ID:     "expected",
		Status: "enabled",
		Source: registry.SkillSource{
			Type:     "company",
			Provider: "test",
			Path:     "skills/expected/SKILL.md",
		},
	}

	_, err := Load(context.Background(), skill, Options{BaseDir: base})
	if err == nil || !strings.Contains(err.Error(), "does not match content name") {
		t.Fatalf("expected mismatch error, got %v", err)
	}
}

func TestLoadRejectsLocalPathTraversal(t *testing.T) {
	skill := registry.Skill{
		ID:     "unsafe-skill",
		Status: "enabled",
		Source: registry.SkillSource{
			Type:     "company",
			Provider: "test",
			Path:     "../outside/SKILL.md",
		},
	}

	_, err := Load(context.Background(), skill, Options{BaseDir: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "escapes") {
		t.Fatalf("expected traversal error, got %v", err)
	}
}
