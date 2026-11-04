package artifact

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
)

type ArchitectureDecisionPayload struct {
	Decision    string   `json:"decision"`
	Rationale   string   `json:"rationale"`
	Constraints []string `json:"constraints,omitempty"`
	Interfaces  []string `json:"interfaces,omitempty"`
	Unknowns    []string `json:"unknowns,omitempty"`
}

type APIContractPayload struct {
	Version    string         `json:"version"`
	Protocol   string         `json:"protocol"`
	Operations []APIOperation `json:"operations"`
}

type APIOperation struct {
	ID             string `json:"id"`
	Method         string `json:"method"`
	Path           string `json:"path"`
	RequestSchema  string `json:"request_schema,omitempty"`
	ResponseSchema string `json:"response_schema,omitempty"`
}

type VerificationReportPayload struct {
	Status string              `json:"status"`
	Checks []VerificationCheck `json:"checks"`
}

type VerificationCheck struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	Summary  string `json:"summary"`
	Evidence string `json:"evidence,omitempty"`
}

func validateTypedPayload(kind Type, payload json.RawMessage) error {
	switch kind {
	case TypeArchitectureDecision:
		var value ArchitectureDecisionPayload
		if err := decodeStrict(payload, &value); err != nil {
			return fmt.Errorf("architecture-decision payload: %w", err)
		}
		return validateArchitectureDecision(value)
	case TypeAPIContract:
		var value APIContractPayload
		if err := decodeStrict(payload, &value); err != nil {
			return fmt.Errorf("api-contract payload: %w", err)
		}
		return validateAPIContract(value)
	case TypeVerificationReport:
		var value VerificationReportPayload
		if err := decodeStrict(payload, &value); err != nil {
			return fmt.Errorf("verification-report payload: %w", err)
		}
		return validateVerificationReport(value)
	default:
		return nil
	}
}

func decodeStrict(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if decoder.More() {
		return errors.New("multiple JSON values are not allowed")
	}
	return nil
}

func validateArchitectureDecision(value ArchitectureDecisionPayload) error {
	if err := requiredBounded("decision", value.Decision, 4000); err != nil {
		return err
	}
	if err := requiredBounded("rationale", value.Rationale, 8000); err != nil {
		return err
	}
	if err := validateStringList("constraints", value.Constraints, 50, 2000); err != nil {
		return err
	}
	if err := validateStringList("interfaces", value.Interfaces, 50, 2000); err != nil {
		return err
	}
	if err := validateStringList("unknowns", value.Unknowns, 50, 2000); err != nil {
		return err
	}
	return nil
}

func validateAPIContract(value APIContractPayload) error {
	if err := requiredBounded("version", value.Version, 100); err != nil {
		return err
	}
	protocol := strings.ToLower(strings.TrimSpace(value.Protocol))
	switch protocol {
	case "http", "https", "rest":
	default:
		return fmt.Errorf("protocol must be http, https, or rest")
	}
	if len(value.Operations) == 0 {
		return errors.New("operations must not be empty")
	}
	if len(value.Operations) > 200 {
		return errors.New("operations exceeds 200 entries")
	}
	seen := map[string]bool{}
	for index, operation := range value.Operations {
		if err := requiredBounded(fmt.Sprintf("operations[%d].id", index), operation.ID, 200); err != nil {
			return err
		}
		if seen[operation.ID] {
			return fmt.Errorf("duplicate operation id: %s", operation.ID)
		}
		seen[operation.ID] = true
		method := strings.ToUpper(strings.TrimSpace(operation.Method))
		switch method {
		case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead, http.MethodOptions:
		default:
			return fmt.Errorf("unsupported HTTP method for operation %s: %s", operation.ID, operation.Method)
		}
		path := strings.TrimSpace(operation.Path)
		if path == "" || !strings.HasPrefix(path, "/") || len(path) > 1000 {
			return fmt.Errorf("operation %s path must be an absolute API path", operation.ID)
		}
		if strings.ContainsAny(path, "\r\n") {
			return fmt.Errorf("operation %s path must be one line", operation.ID)
		}
		if len(operation.RequestSchema) > 16000 || len(operation.ResponseSchema) > 16000 {
			return fmt.Errorf("operation %s schema summary exceeds 16 KiB", operation.ID)
		}
	}
	return nil
}

func validateVerificationReport(value VerificationReportPayload) error {
	switch strings.ToLower(strings.TrimSpace(value.Status)) {
	case "passed", "failed", "partial", "infrastructure":
	default:
		return errors.New("verification status must be passed, failed, partial, or infrastructure")
	}
	if len(value.Checks) == 0 {
		return errors.New("verification checks must not be empty")
	}
	if len(value.Checks) > 500 {
		return errors.New("verification checks exceeds 500 entries")
	}
	seen := map[string]bool{}
	for index, check := range value.Checks {
		if err := requiredBounded(fmt.Sprintf("checks[%d].id", index), check.ID, 300); err != nil {
			return err
		}
		if seen[check.ID] {
			return fmt.Errorf("duplicate verification check id: %s", check.ID)
		}
		seen[check.ID] = true
		switch strings.ToLower(strings.TrimSpace(check.Status)) {
		case "passed", "failed", "skipped", "partial", "infrastructure":
		default:
			return fmt.Errorf("unsupported status for verification check %s: %s", check.ID, check.Status)
		}
		if err := requiredBounded(fmt.Sprintf("checks[%d].summary", index), check.Summary, 4000); err != nil {
			return err
		}
		if len(check.Evidence) > 8000 {
			return fmt.Errorf("verification check %s evidence exceeds 8000 characters", check.ID)
		}
	}
	return nil
}

func requiredBounded(name, value string, max int) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("%s is required", name)
	}
	if len(value) > max {
		return fmt.Errorf("%s exceeds %d characters", name, max)
	}
	return nil
}

func validateStringList(name string, values []string, maxItems, maxLength int) error {
	if len(values) > maxItems {
		return fmt.Errorf("%s exceeds %d entries", name, maxItems)
	}
	normalized := append([]string(nil), values...)
	sort.Strings(normalized)
	for index, value := range normalized {
		value = strings.TrimSpace(value)
		if value == "" {
			return fmt.Errorf("%s contains an empty entry", name)
		}
		if len(value) > maxLength {
			return fmt.Errorf("%s entry %d exceeds %d characters", name, index, maxLength)
		}
	}
	return nil
}
