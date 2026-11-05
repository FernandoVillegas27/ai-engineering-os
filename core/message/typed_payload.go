package message

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

const PayloadSchemaVersion = 1

type HandoffPayload struct {
	SchemaVersion int    `json:"schema_version"`
	FromTask      string `json:"from_task"`
	ToTask        string `json:"to_task"`
	Summary       string `json:"summary"`
}

type QuestionPayload struct {
	SchemaVersion int      `json:"schema_version"`
	Question      string   `json:"question"`
	Context       string   `json:"context,omitempty"`
	Options       []string `json:"options,omitempty"`
}

type AnswerPayload struct {
	SchemaVersion int    `json:"schema_version"`
	Answer        string `json:"answer"`
	Rationale     string `json:"rationale,omitempty"`
}

type BlockerPayload struct {
	SchemaVersion int    `json:"schema_version"`
	Reason        string `json:"reason"`
	Evidence      string `json:"evidence,omitempty"`
	Needs         string `json:"needs,omitempty"`
}

type DecisionPayload struct {
	SchemaVersion int    `json:"schema_version"`
	Decision      string `json:"decision"`
	Rationale     string `json:"rationale,omitempty"`
}

type StatusPayload struct {
	SchemaVersion int    `json:"schema_version"`
	State         string `json:"state"`
	Detail        string `json:"detail,omitempty"`
}

func PayloadSchemaID(kind Kind, payload json.RawMessage) string {
	if !payloadHasSchemaVersion(payload) {
		return "message/" + string(kind) + "/legacy-v1"
	}
	switch kind {
	case KindHandoff, KindQuestion, KindAnswer, KindBlocker, KindDecision, KindStatus:
		return "message/" + string(kind) + "/v1"
	default:
		return "message/" + string(kind) + "/generic-v1"
	}
}

func validateTypedPayload(kind Kind, payload json.RawMessage) error {
	if !payloadHasSchemaVersion(payload) {
		return nil // persisted schema-v1 messages predate typed payload contracts.
	}
	switch kind {
	case KindHandoff:
		var value HandoffPayload
		if err := decodePayloadStrict(payload, &value); err != nil { return fmt.Errorf("handoff payload: %w", err) }
		if err := validatePayloadVersion(value.SchemaVersion); err != nil { return err }
		if err := requiredPayloadText("from_task", value.FromTask, 300); err != nil { return err }
		if err := requiredPayloadText("to_task", value.ToTask, 300); err != nil { return err }
		return requiredPayloadText("summary", value.Summary, 8000)
	case KindQuestion:
		var value QuestionPayload
		if err := decodePayloadStrict(payload, &value); err != nil { return fmt.Errorf("question payload: %w", err) }
		if err := validatePayloadVersion(value.SchemaVersion); err != nil { return err }
		if err := requiredPayloadText("question", value.Question, 8000); err != nil { return err }
		if len(value.Context) > 12000 { return errors.New("question context exceeds 12000 characters") }
		if len(value.Options) > 50 { return errors.New("question options exceeds 50 entries") }
		for _, option := range value.Options {
			if err := requiredPayloadText("question option", option, 4000); err != nil { return err }
		}
		return nil
	case KindAnswer:
		var value AnswerPayload
		if err := decodePayloadStrict(payload, &value); err != nil { return fmt.Errorf("answer payload: %w", err) }
		if err := validatePayloadVersion(value.SchemaVersion); err != nil { return err }
		if err := requiredPayloadText("answer", value.Answer, 12000); err != nil { return err }
		if len(value.Rationale) > 12000 { return errors.New("answer rationale exceeds 12000 characters") }
		return nil
	case KindBlocker:
		var value BlockerPayload
		if err := decodePayloadStrict(payload, &value); err != nil { return fmt.Errorf("blocker payload: %w", err) }
		if err := validatePayloadVersion(value.SchemaVersion); err != nil { return err }
		if err := requiredPayloadText("reason", value.Reason, 8000); err != nil { return err }
		if len(value.Evidence) > 12000 { return errors.New("blocker evidence exceeds 12000 characters") }
		if len(value.Needs) > 8000 { return errors.New("blocker needs exceeds 8000 characters") }
		return nil
	case KindDecision:
		var value DecisionPayload
		if err := decodePayloadStrict(payload, &value); err != nil { return fmt.Errorf("decision payload: %w", err) }
		if err := validatePayloadVersion(value.SchemaVersion); err != nil { return err }
		if err := requiredPayloadText("decision", value.Decision, 8000); err != nil { return err }
		if len(value.Rationale) > 12000 { return errors.New("decision rationale exceeds 12000 characters") }
		return nil
	case KindStatus:
		var value StatusPayload
		if err := decodePayloadStrict(payload, &value); err != nil { return fmt.Errorf("status payload: %w", err) }
		if err := validatePayloadVersion(value.SchemaVersion); err != nil { return err }
		if err := requiredPayloadText("state", value.State, 300); err != nil { return err }
		if len(value.Detail) > 8000 { return errors.New("status detail exceeds 8000 characters") }
		return nil
	default:
		return nil
	}
}

func decodePayloadStrict(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil { return err }
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil { return errors.New("multiple JSON values are not allowed") }
		return fmt.Errorf("invalid trailing JSON: %w", err)
	}
	return nil
}

func payloadHasSchemaVersion(data []byte) bool {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(data, &envelope); err != nil { return false }
	_, ok := envelope["schema_version"]
	return ok
}

func validatePayloadVersion(version int) error {
	if version != PayloadSchemaVersion {
		return fmt.Errorf("unsupported message payload schema version: %d", version)
	}
	return nil
}

func requiredPayloadText(name, value string, max int) error {
	value = strings.TrimSpace(value)
	if value == "" { return fmt.Errorf("%s is required", name) }
	if len(value) > max { return fmt.Errorf("%s exceeds %d characters", name, max) }
	return nil
}
