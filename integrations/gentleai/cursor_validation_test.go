package gentleai

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestValidateCursorHealthyProjection(t *testing.T) {
	home := t.TempDir()
	cursor := filepath.Join(home, ".cursor")

	engram := filepath.Join(home, "bin", "engram.exe")
	writeTestFile(t, engram, "binary")

	writeTestFile(t, filepath.Join(cursor, "rules", "gentle-ai.mdc"), `---
alwaysApply: true
---
# Organic Driven Development (ODD)
Current workflow guidance.
`)

	writeTestFile(t, filepath.Join(cursor, "mcp.json"), `{
  "mcpServers": {
    "engram": {
      "command": "`+escapeJSONPath(engram)+`",
      "args": ["mcp", "--tools=agent"]
    },
    "context7": {
      "url": "https://mcp.context7.com/mcp"
    }
  }
}`)

	for _, name := range []string{"jd-judge-a.md", "jd-judge-b.md", "jd-fix-agent.md"} {
		writeTestFile(t, filepath.Join(cursor, "agents", name), "agent")
	}

	report := ValidateCursor(home)
	if report.HasErrors() {
		t.Fatalf("expected healthy projection, got %#v", report)
	}
	for _, check := range report.Checks {
		if check.Level != ValidationOK {
			t.Fatalf("check %s = %s (%s)", check.ID, check.Level, check.Message)
		}
	}
}

func TestValidateCursorDetectsActiveLegacySDDRule(t *testing.T) {
	home := t.TempDir()
	cursor := filepath.Join(home, ".cursor")

	writeTestFile(t, filepath.Join(cursor, "rules", "gentle-ai.mdc"), `---
alwaysApply: true
---
Organic Driven Development
<!-- gentle-ai:sdd-orchestrator -->
SDD Session Preflight
`)

	report := ValidateCursor(home)
	var found bool
	for _, check := range report.Checks {
		if check.ID == "cursor-rule" {
			found = true
			if check.Level != ValidationError {
				t.Fatalf("cursor-rule level = %s, want error", check.Level)
			}
		}
	}
	if !found {
		t.Fatal("cursor-rule check missing")
	}
}

func TestValidateCursorWarnsOnLegacySDDAssets(t *testing.T) {
	home := t.TempDir()
	cursor := filepath.Join(home, ".cursor")

	writeTestFile(t, filepath.Join(cursor, "skills", "sdd-apply", "SKILL.md"), "legacy")
	writeTestFile(t, filepath.Join(cursor, "agents", "sdd-verify.md"), "legacy")

	check := validateLegacySDD(cursor)
	if check.Level != ValidationWarn {
		t.Fatalf("level = %s, want warn", check.Level)
	}
}

func escapeJSONPath(path string) string {
	result := ""
	for _, r := range path {
		if r == '\\' {
			result += "\\\\"
		} else {
			result += string(r)
		}
	}
	return result
}
