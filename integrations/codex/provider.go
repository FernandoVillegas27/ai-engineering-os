package codex

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/FernandoVillegas27/ai-engineering-os/core/modelprovider"
	"github.com/FernandoVillegas27/ai-engineering-os/integrations/commandmodel"
)

const (
	ProviderID = "codex-cli"
	schemaFile = "classification.schema.json"
	resultFile = "classification.json"
)

type Adapter struct {
	Binary string
	Model  string
}

func New() commandmodel.Provider {
	return commandmodel.Provider{
		Adapter: Adapter{Binary: "codex"},
	}
}

func (a Adapter) ID() string {
	return ProviderID
}

func (a Adapter) Build(request modelprovider.ClassificationRequest, workDir string) (commandmodel.Invocation, error) {
	if strings.TrimSpace(request.Task) == "" {
		return commandmodel.Invocation{}, fmt.Errorf("classification task is required")
	}
	if len(request.AllowedIntents) == 0 || len(request.AllowedCapabilities) == 0 {
		return commandmodel.Invocation{}, fmt.Errorf("allowed intents and capabilities are required")
	}

	schemaPath := filepath.Join(workDir, schemaFile)
	resultPath := filepath.Join(workDir, resultFile)

	schema, err := classificationSchema(request)
	if err != nil {
		return commandmodel.Invocation{}, err
	}
	if err := os.WriteFile(schemaPath, schema, 0o600); err != nil {
		return commandmodel.Invocation{}, fmt.Errorf("write Codex output schema: %w", err)
	}

	binary := strings.TrimSpace(a.Binary)
	if binary == "" {
		binary = "codex"
	}

	args := []string{
		"exec",
		"--sandbox", "read-only",
		"--ask-for-approval", "never",
		"--output-schema", schemaPath,
		"--output-last-message", resultPath,
	}
	if model := strings.TrimSpace(a.Model); model != "" {
		args = append(args, "--model", model)
	}
	args = append(args, "-")

	return commandmodel.Invocation{
		Command: binary,
		Args:    args,
		Stdin:   classificationPrompt(request),
		Dir:     workDir,
	}, nil
}

func (a Adapter) Decode(workDir string) (modelprovider.ClassificationResponse, error) {
	data, err := os.ReadFile(filepath.Join(workDir, resultFile))
	if err != nil {
		return modelprovider.ClassificationResponse{}, fmt.Errorf("read Codex classification result: %w", err)
	}

	var response modelprovider.ClassificationResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return modelprovider.ClassificationResponse{}, fmt.Errorf("parse Codex classification JSON: %w", err)
	}
	return response, nil
}

func classificationPrompt(request modelprovider.ClassificationRequest) string {
	var b strings.Builder
	b.WriteString("Classify the software-engineering task using only the allowed taxonomy below.\n")
	b.WriteString("Do not invent intents or capabilities. If the task is genuinely underspecified, set can_classify=false, intent=\"\", capabilities=[], and ask exactly one focused clarification question.\n")
	b.WriteString("Return only the structured output required by the supplied JSON schema.\n\n")
	b.WriteString("Task:\n")
	b.WriteString(request.Task)
	b.WriteString("\n\nAllowed intents:\n")
	for _, intent := range request.AllowedIntents {
		b.WriteString("- ")
		b.WriteString(string(intent))
		b.WriteString("\n")
	}
	b.WriteString("\nAllowed capabilities:\n")
	for _, capability := range request.AllowedCapabilities {
		b.WriteString("- ")
		b.WriteString(capability)
		b.WriteString("\n")
	}
	if context := strings.TrimSpace(request.ProjectContext); context != "" {
		b.WriteString("\nProject context:\n")
		b.WriteString(context)
		b.WriteString("\n")
	}
	return b.String()
}

func classificationSchema(request modelprovider.ClassificationRequest) ([]byte, error) {
	intents := []string{""}
	for _, intent := range request.AllowedIntents {
		intents = append(intents, string(intent))
	}

	schema := map[string]any{
		"type": "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"intent": map[string]any{
				"type": "string",
				"enum": intents,
			},
			"capabilities": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "string",
					"enum": request.AllowedCapabilities,
				},
				"uniqueItems": true,
			},
			"can_classify": map[string]any{
				"type": "boolean",
			},
			"clarification_question": map[string]any{
				"type": "string",
			},
			"rationale": map[string]any{
				"type": "string",
			},
		},
		"required": []string{
			"intent",
			"capabilities",
			"can_classify",
			"clarification_question",
			"rationale",
		},
	}

	data, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal Codex output schema: %w", err)
	}
	return data, nil
}
