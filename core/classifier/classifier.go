package classifier

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/FernandoVillegas27/ai-engineering-os/core/orchestration"
	"github.com/FernandoVillegas27/ai-engineering-os/core/registry"
)

type Confidence string

const (
	ConfidenceHigh      Confidence = "high"
	ConfidenceAmbiguous Confidence = "ambiguous"
)

type Result struct {
	Task         string
	Intent       orchestration.Intent
	Capabilities []string
	Confidence   Confidence
	NeedsAI      bool
	Reasons      []string
}

func Classify(task string, capabilities registry.CapabilityRegistry) (Result, error) {
	task = strings.TrimSpace(task)
	if task == "" {
		return Result{}, fmt.Errorf("task cannot be empty")
	}

	normalized := normalize(task)
	intent, intentReason := detectIntent(normalized)
	matches, reasons := detectCapabilities(normalized, capabilities)

	result := Result{
		Task:         task,
		Intent:       intent,
		Capabilities: matches,
		Reasons:      append([]string{}, reasons...),
	}

	if intentReason != "" {
		result.Reasons = append(result.Reasons, intentReason)
	}

	if intent == "" || len(matches) == 0 {
		result.Confidence = ConfidenceAmbiguous
		result.NeedsAI = true
		sort.Strings(result.Reasons)
		return result, nil
	}

	result.Confidence = ConfidenceHigh
	result.NeedsAI = false
	sort.Strings(result.Reasons)
	return result, nil
}

func detectIntent(text string) (orchestration.Intent, string) {
	type intentRule struct {
		Intent orchestration.Intent
		Terms  []string
	}

	rules := []intentRule{
		{Intent: orchestration.IntentReview, Terms: []string{
			"revisar", "revisa", "review", "code review", "auditar", "audita", "inspeccionar cambio", "revisar cambio",
		}},
		{Intent: orchestration.IntentVerify, Terms: []string{
			"verificar", "verifica", "validar", "valida", "probar", "prueba", "testear", "test", "comprobar",
		}},
		{Intent: orchestration.IntentResearch, Terms: []string{
			"investigar", "investiga", "research", "averiguar", "comparar", "compara", "evaluar opciones", "analizar opciones",
		}},
		{Intent: orchestration.IntentDesign, Terms: []string{
			"disenar", "disena", "design", "arquitectar", "arquitectura", "modelar", "modelo de datos", "definir arquitectura",
		}},
		{Intent: orchestration.IntentImplement, Terms: []string{
			"implementar", "implementa", "crear", "crea", "agregar", "agrega", "anadir", "construir", "build",
			"develop", "desarrollar", "desarrolla", "modificar", "modifica", "cambiar", "cambia", "corregir", "corrige",
			"fix", "actualizar", "actualiza",
		}},
	}

	for _, rule := range rules {
		if term, ok := containsAny(text, rule.Terms); ok {
			return rule.Intent, "intent:" + string(rule.Intent) + " via " + term
		}
	}

	return "", ""
}

func detectCapabilities(text string, registry registry.CapabilityRegistry) ([]string, []string) {
	hints := map[string][]string{
		"backend-development": {
			"backend", "server side", "server-side", "servicio", "service layer", "logica de negocio",
		},
		"api-development": {
			"api", "endpoint", "rest", "rest controller", "controller", "http endpoint", "webhook",
		},
		"frontend-development": {
			"frontend", "ui", "interfaz", "pantalla", "componente", "component", "angular", "react", "css",
		},
		"database-design": {
			"base de datos", "database", "schema", "esquema", "tabla", "table", "migration", "migracion", "sql", "query",
		},
		"code-review": {
			"code review", "review", "revisar codigo", "revisar cambio", "pull request", "pr", "diff",
		},
		"automated-testing": {
			"test", "tests", "testing", "prueba automatizada", "pruebas automatizadas", "e2e", "unit test", "integration test",
		},
		"security-review": {
			"security", "seguridad", "vulnerabilidad", "auth", "authentication", "authorization", "permiso", "permission",
		},
		"technical-research": {
			"research", "investigar", "documentacion", "documentation", "comparar opciones", "evaluar tecnologia", "averiguar",
		},
	}

	ids := make([]string, 0, len(registry.Capabilities))
	for id := range registry.Capabilities {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	var matched []string
	var reasons []string

	for _, id := range ids {
		capability := registry.Capabilities[id]
		if capability.Status != "active" {
			continue
		}

		terms := []string{id}
		terms = append(terms, capability.Aliases...)
		terms = append(terms, hints[id]...)

		if term, ok := containsAny(text, terms); ok {
			matched = append(matched, id)
			reasons = append(reasons, "capability:"+id+" via "+term)
		}
	}

	return uniqueSorted(matched), reasons
}

func containsAny(text string, terms []string) (string, bool) {
	for _, term := range terms {
		normalizedTerm := normalize(term)
		if normalizedTerm == "" {
			continue
		}
		if containsPhrase(text, normalizedTerm) {
			return normalizedTerm, true
		}
	}
	return "", false
}

func containsPhrase(text, phrase string) bool {
	if strings.Contains(phrase, " ") || strings.Contains(phrase, "-") {
		return strings.Contains(text, phrase)
	}

	fields := strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-'
	})
	for _, field := range fields {
		if field == phrase {
			return true
		}
	}
	return false
}

func normalize(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer(
		"á", "a",
		"é", "e",
		"í", "i",
		"ó", "o",
		"ú", "u",
		"ü", "u",
		"ñ", "n",
	)
	return strings.Join(strings.Fields(replacer.Replace(value)), " ")
}

func uniqueSorted(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
