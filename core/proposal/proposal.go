package proposal

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
)

type FileChange struct {
	Path      string
	Action    string
	Rationale string
}

type Risk struct {
	Severity string
	Detail   string
}

type Proposal struct {
	Summary      string
	Files        []FileChange
	Patch        string
	Risks        []Risk
	Verification []string
}

type Runtime interface {
	ID() string
	Propose(ctx context.Context, unit execution.WorkUnit) (Proposal, error)
}

type Generator struct {
	Runtime Runtime
}

func (g Generator) Generate(ctx context.Context, unit execution.WorkUnit) (Proposal, error) {
	if g.Runtime == nil {
		return Proposal{}, fmt.Errorf("proposal runtime is required")
	}
	if err := execution.ValidateReadOnlyUnit(unit); err != nil {
		return Proposal{}, err
	}

	proposal, err := g.Runtime.Propose(ctx, unit)
	if err != nil {
		return Proposal{}, fmt.Errorf("proposal runtime %s: %w", g.Runtime.ID(), err)
	}
	if strings.TrimSpace(proposal.Summary) == "" {
		return Proposal{}, fmt.Errorf("proposal runtime %s returned an empty summary", g.Runtime.ID())
	}
	if err := ValidateTaskAlignment(unit.Task, proposal); err != nil {
		return Proposal{}, fmt.Errorf("proposal runtime %s returned a task-misaligned proposal: %w", g.Runtime.ID(), err)
	}
	return proposal, nil
}


var taskTokenPattern = regexp.MustCompile("[[:alnum:]_]+")

func ValidateTaskAlignment(task string, p Proposal) error {
	anchors := taskAnchors(task)
	if len(anchors) == 0 {
		return nil
	}

	var b strings.Builder
	b.WriteString(p.Summary)
	b.WriteString(" ")
	for _, file := range p.Files {
		b.WriteString(file.Path)
		b.WriteString(" ")
		b.WriteString(file.Rationale)
		b.WriteString(" ")
	}
	for _, step := range p.Verification {
		b.WriteString(step)
		b.WriteString(" ")
	}
	haystack := normalizeText(b.String())
	proposalTokens := map[string]bool{}
	for _, token := range taskTokenPattern.FindAllString(haystack, -1) {
		proposalTokens[token] = true
	}

	hits := 0
	var missing []string
	var missingRequired []string
	for _, anchor := range anchors {
		if proposalTokens[anchor] {
			hits++
			continue
		}
		missing = append(missing, anchor)
		if isNumericToken(anchor) {
			missingRequired = append(missingRequired, anchor)
		}
	}

	if len(missingRequired) > 0 {
		return fmt.Errorf("required task anchors are missing: %s", strings.Join(missingRequired, ", "))
	}

	required := 1
	if len(anchors) > 1 {
		// Require 75% coverage so generic overlap (for example runtime/codex)
		// cannot authorize a proposal that omits the actual requested concern.
		required = (len(anchors)*3 + 3) / 4
	}
	if hits < required {
		return fmt.Errorf("task anchors matched %d/%d, need at least %d; missing: %s", hits, len(anchors), required, strings.Join(missing, ", "))
	}
	return nil
}

func taskAnchors(task string) []string {
	normalized := normalizeText(task)
	stopwords := map[string]bool{
		"agrega": true, "agregar": true, "anadi": true, "anadir": true, "implementa": true, "implementar": true,
		"crea": true, "crear": true, "modifica": true, "modificar": true, "cambia": true, "cambiar": true,
		"fix": true, "add": true, "create": true, "modify": true, "change": true, "implement": true,
		"para": true, "que": true, "una": true, "uno": true, "unos": true, "unas": true, "del": true, "las": true, "los": true,
		"pueda": true, "puede": true, "puedan": true, "debe": true, "deberia": true, "configurarse": true, "configurar": true,
		"menos": true, "mas": true, "segundo": true, "segundos": true, "second": true, "seconds": true,
		"validacion": true, "validar": true, "validation": true, "validate": true,
		"the": true, "and": true, "for": true, "with": true, "from": true, "into": true, "this": true, "that": true, "than": true,
	}
	seen := map[string]bool{}
	var anchors []string
	for _, token := range taskTokenPattern.FindAllString(normalized, -1) {
		token = strings.Trim(token, "._/:-")
		if token == "" || stopwords[token] {
			continue
		}
		isNumber := true
		for _, r := range token {
			if r < '0' || r > '9' {
				isNumber = false
				break
			}
		}
		if !isNumber && len(token) < 4 {
			continue
		}
		if seen[token] {
			continue
		}
		seen[token] = true
		anchors = append(anchors, token)
	}
	return anchors
}


func isNumericToken(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func normalizeText(value string) string {
	value = strings.ToLower(value)
	replacer := strings.NewReplacer(
		"á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u", "ñ", "n",
	)
	return replacer.Replace(value)
}
