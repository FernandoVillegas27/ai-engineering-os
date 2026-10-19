package gentleai

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type ValidationLevel string

const (
	ValidationOK    ValidationLevel = "ok"
	ValidationWarn  ValidationLevel = "warn"
	ValidationError ValidationLevel = "error"
)

type ValidationCheck struct {
	ID      string
	Label   string
	Level   ValidationLevel
	Message string
	Hint    string
}

type ValidationReport struct {
	Checks []ValidationCheck
}

func (r ValidationReport) HasErrors() bool {
	for _, check := range r.Checks {
		if check.Level == ValidationError {
			return true
		}
	}
	return false
}

func ValidateCursor(home string) ValidationReport {
	cursorRoot := filepath.Join(home, ".cursor")

	checks := []ValidationCheck{
		validateCursorRule(cursorRoot),
		validateCursorEngram(cursorRoot),
		validateCursorContext7(cursorRoot),
		validateCursorJudgmentDay(cursorRoot),
	}

	checks = append(checks, validateLegacySDD(cursorRoot))

	return ValidationReport{Checks: checks}
}

func validateCursorRule(cursorRoot string) ValidationCheck {
	path := filepath.Join(cursorRoot, "rules", "gentle-ai.mdc")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return ValidationCheck{
			ID:      "cursor-rule",
			Label:   "Gentle Rule",
			Level:   ValidationError,
			Message: "No existe ~/.cursor/rules/gentle-ai.mdc.",
			Hint:    "Ejecutá gentle-ai sync --agent cursor.",
		}
	}
	if err != nil {
		return ValidationCheck{
			ID:      "cursor-rule",
			Label:   "Gentle Rule",
			Level:   ValidationError,
			Message: "No se pudo leer gentle-ai.mdc.",
			Hint:    err.Error(),
		}
	}

	content := strings.ToLower(string(data))
	if !strings.Contains(content, "alwaysapply: true") {
		return ValidationCheck{
			ID:      "cursor-rule",
			Label:   "Gentle Rule",
			Level:   ValidationError,
			Message: "La regla de Gentle-AI no está marcada alwaysApply.",
			Hint:    "Ejecutá gentle-ai sync --agent cursor.",
		}
	}

	hasODD := strings.Contains(content, "organic driven development") ||
		strings.Contains(content, "organic-driven development") ||
		strings.Contains(content, "odd")
	if !hasODD {
		return ValidationCheck{
			ID:      "cursor-rule",
			Label:   "Gentle Rule",
			Level:   ValidationWarn,
			Message: "La regla está activa, pero no se detectó guidance ODD.",
			Hint:    "Revisá ~/.cursor/rules/gentle-ai.mdc antes de usar routing automático.",
		}
	}

	if strings.Contains(content, "gentle-ai:sdd-orchestrator") ||
		strings.Contains(content, "sdd session preflight") {
		return ValidationCheck{
			ID:      "cursor-rule",
			Label:   "Gentle Rule",
			Level:   ValidationError,
			Message: "La regla activa todavía contiene routing SDD legacy.",
			Hint:    "No uses esa regla hasta retirar el bloque SDD o regenerarla con Gentle-AI v4.",
		}
	}

	return ValidationCheck{
		ID:      "cursor-rule",
		Label:   "Gentle Rule",
		Level:   ValidationOK,
		Message: "Regla alwaysApply activa con routing ODD.",
	}
}

type cursorMCPConfig struct {
	MCPServers map[string]json.RawMessage `json:"mcpServers"`
}

type cursorLocalMCP struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

func readCursorMCP(cursorRoot string) (cursorMCPConfig, error) {
	path := filepath.Join(cursorRoot, "mcp.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return cursorMCPConfig{}, err
	}

	var config cursorMCPConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return cursorMCPConfig{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return config, nil
}

func validateCursorEngram(cursorRoot string) ValidationCheck {
	config, err := readCursorMCP(cursorRoot)
	if os.IsNotExist(err) {
		return ValidationCheck{
			ID:      "cursor-engram",
			Label:   "Engram MCP",
			Level:   ValidationError,
			Message: "No existe ~/.cursor/mcp.json.",
			Hint:    "Configurá Engram MCP para Cursor.",
		}
	}
	if err != nil {
		return ValidationCheck{
			ID:      "cursor-engram",
			Label:   "Engram MCP",
			Level:   ValidationError,
			Message: "No se pudo validar Cursor MCP.",
			Hint:    err.Error(),
		}
	}

	raw, ok := config.MCPServers["engram"]
	if !ok {
		return ValidationCheck{
			ID:      "cursor-engram",
			Label:   "Engram MCP",
			Level:   ValidationError,
			Message: "Cursor no tiene un servidor MCP llamado engram.",
			Hint:    "Ejecutá engram setup cursor o gentle-ai sync --agent cursor.",
		}
	}

	var server cursorLocalMCP
	if err := json.Unmarshal(raw, &server); err != nil {
		return ValidationCheck{
			ID:      "cursor-engram",
			Label:   "Engram MCP",
			Level:   ValidationError,
			Message: "La entrada MCP de Engram no tiene formato local esperado.",
			Hint:    err.Error(),
		}
	}

	if strings.TrimSpace(server.Command) == "" {
		return ValidationCheck{
			ID:      "cursor-engram",
			Label:   "Engram MCP",
			Level:   ValidationError,
			Message: "La entrada MCP de Engram no define command.",
		}
	}

	if _, err := os.Stat(server.Command); err != nil {
		return ValidationCheck{
			ID:      "cursor-engram",
			Label:   "Engram MCP",
			Level:   ValidationError,
			Message: "El ejecutable configurado para Engram no existe.",
			Hint:    server.Command,
		}
	}

	args := strings.Join(server.Args, " ")
	if !strings.Contains(args, "mcp") || !strings.Contains(args, "--tools=agent") {
		return ValidationCheck{
			ID:      "cursor-engram",
			Label:   "Engram MCP",
			Level:   ValidationWarn,
			Message: "Engram existe, pero sus argumentos MCP no coinciden con el perfil de agente esperado.",
			Hint:    args,
		}
	}

	return ValidationCheck{
		ID:      "cursor-engram",
		Label:   "Engram MCP",
		Level:   ValidationOK,
		Message: "Engram MCP apunta a un ejecutable válido.",
	}
}

func validateCursorContext7(cursorRoot string) ValidationCheck {
	config, err := readCursorMCP(cursorRoot)
	if err != nil {
		return ValidationCheck{
			ID:      "cursor-context7",
			Label:   "Context7 MCP",
			Level:   ValidationError,
			Message: "No se pudo validar Cursor MCP para Context7.",
			Hint:    err.Error(),
		}
	}

	if _, ok := config.MCPServers["context7"]; !ok {
		return ValidationCheck{
			ID:      "cursor-context7",
			Label:   "Context7 MCP",
			Level:   ValidationError,
			Message: "Cursor no tiene un servidor MCP llamado context7.",
			Hint:    "Ejecutá gentle-ai sync --agent cursor o restaurá tu entrada Context7.",
		}
	}

	return ValidationCheck{
		ID:      "cursor-context7",
		Label:   "Context7 MCP",
		Level:   ValidationOK,
		Message: "Context7 MCP está registrado en Cursor.",
	}
}

func validateCursorJudgmentDay(cursorRoot string) ValidationCheck {
	agentsRoot := filepath.Join(cursorRoot, "agents")
	required := []string{
		"jd-judge-a.md",
		"jd-judge-b.md",
		"jd-fix-agent.md",
	}

	var missing []string
	for _, name := range required {
		if _, err := os.Stat(filepath.Join(agentsRoot, name)); err != nil {
			missing = append(missing, name)
		}
	}

	if len(missing) > 0 {
		return ValidationCheck{
			ID:      "cursor-review-agents",
			Label:   "Review Agents",
			Level:   ValidationWarn,
			Message: "Faltan agentes de revisión administrados por Gentle-AI.",
			Hint:    strings.Join(missing, ", "),
		}
	}

	return ValidationCheck{
		ID:      "cursor-review-agents",
		Label:   "Review Agents",
		Level:   ValidationOK,
		Message: "Judgment Day agents presentes.",
	}
}

func validateLegacySDD(cursorRoot string) ValidationCheck {
	var remnants []string

	skillsRoot := filepath.Join(cursorRoot, "skills")
	if entries, err := os.ReadDir(skillsRoot); err == nil {
		for _, entry := range entries {
			if strings.HasPrefix(strings.ToLower(entry.Name()), "sdd-") {
				remnants = append(remnants, filepath.Join("skills", entry.Name()))
			}
		}
	}

	agentsRoot := filepath.Join(cursorRoot, "agents")
	if entries, err := os.ReadDir(agentsRoot); err == nil {
		for _, entry := range entries {
			if strings.HasPrefix(strings.ToLower(entry.Name()), "sdd-") {
				remnants = append(remnants, filepath.Join("agents", entry.Name()))
			}
		}
	}

	sort.Strings(remnants)

	if len(remnants) == 0 {
		return ValidationCheck{
			ID:      "cursor-legacy-sdd",
			Label:   "Legacy SDD",
			Level:   ValidationOK,
			Message: "No se detectaron assets SDD legacy en Cursor.",
		}
	}

	return ValidationCheck{
		ID:      "cursor-legacy-sdd",
		Label:   "Legacy SDD",
		Level:   ValidationWarn,
		Message: fmt.Sprintf("Quedaron %d asset(s) SDD legacy preservados.", len(remnants)),
		Hint:    strings.Join(remnants, ", "),
	}
}
