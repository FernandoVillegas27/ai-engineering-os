package artifact

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
)

const LifecyclePayloadSchemaVersion = 1

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

type TaskBriefPayload struct {
	SchemaVersion      int      `json:"schema_version"`
	Objective          string   `json:"objective"`
	AcceptanceCriteria []string `json:"acceptance_criteria"`
	Constraints        []string `json:"constraints,omitempty"`
	OutOfScope         []string `json:"out_of_scope,omitempty"`
}

type ResearchNotePayload struct {
	SchemaVersion int      `json:"schema_version"`
	Finding       string   `json:"finding"`
	Evidence      []string `json:"evidence,omitempty"`
	Unknowns      []string `json:"unknowns,omitempty"`
}

type ImplementationPlanPayload struct {
	SchemaVersion int                `json:"schema_version"`
	Goal          string             `json:"goal"`
	Steps         []ImplementationStep `json:"steps"`
	Risks         []string           `json:"risks,omitempty"`
	OpenQuestions []string           `json:"open_questions,omitempty"`
}

type ImplementationStep struct {
	ID          string   `json:"id"`
	Summary     string   `json:"summary"`
	DependsOn   []string `json:"depends_on,omitempty"`
	Files       []string `json:"files,omitempty"`
	Verification string  `json:"verification,omitempty"`
}

type ChangeProposalPayload struct {
	SchemaVersion int      `json:"schema_version"`
	Objective     string   `json:"objective"`
	Changes       []string `json:"changes"`
	Files         []string `json:"files,omitempty"`
	Risks         []string `json:"risks,omitempty"`
}

type CodePatchReferencePayload struct {
	SchemaVersion int      `json:"schema_version"`
	Reference     string   `json:"reference"`
	Files         []string `json:"files"`
	BaseReference string   `json:"base_reference,omitempty"`
}

type TestPlanPayload struct {
	SchemaVersion int        `json:"schema_version"`
	Strategy      string     `json:"strategy"`
	Cases         []TestCase `json:"cases"`
}

type TestCase struct {
	ID       string `json:"id"`
	Scope    string `json:"scope"`
	Expected string `json:"expected"`
}

type ReviewReportPayload struct {
	SchemaVersion int             `json:"schema_version"`
	Verdict       string          `json:"verdict"`
	Findings      []ReviewFinding `json:"findings,omitempty"`
}

type ReviewFinding struct {
	ID       string `json:"id"`
	Severity string `json:"severity"`
	Summary  string `json:"summary"`
	Evidence string `json:"evidence,omitempty"`
}

type RiskAssessmentPayload struct {
	SchemaVersion int        `json:"schema_version"`
	Level         string     `json:"level"`
	Risks         []RiskItem `json:"risks,omitempty"`
}

type RiskItem struct {
	ID         string `json:"id"`
	Severity   string `json:"severity"`
	Summary    string `json:"summary"`
	Mitigation string `json:"mitigation,omitempty"`
}

type QuestionPayload struct {
	SchemaVersion int      `json:"schema_version"`
	Question      string   `json:"question"`
	Context       string   `json:"context,omitempty"`
	Options       []string `json:"options,omitempty"`
}

type DecisionPayload struct {
	SchemaVersion int      `json:"schema_version"`
	Decision      string   `json:"decision"`
	Rationale     string   `json:"rationale"`
	Consequences  []string `json:"consequences,omitempty"`
}

type HandoffPayload struct {
	SchemaVersion int      `json:"schema_version"`
	TaskID        string   `json:"task_id"`
	AgentID       string   `json:"agent_id"`
	Summary       string   `json:"summary"`
	ArtifactRefs  []string `json:"artifact_refs,omitempty"`
	MessageRefs   []string `json:"message_refs,omitempty"`
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
	case TypeTaskBrief:
		var value TaskBriefPayload
		if err := decodeStrict(payload, &value); err != nil { return fmt.Errorf("task-brief payload: %w", err) }
		return validateTaskBrief(value)
	case TypeResearchNote:
		var value ResearchNotePayload
		if err := decodeStrict(payload, &value); err != nil { return fmt.Errorf("research-note payload: %w", err) }
		return validateResearchNote(value)
	case TypeImplementationPlan:
		var value ImplementationPlanPayload
		if err := decodeStrict(payload, &value); err != nil { return fmt.Errorf("implementation-plan payload: %w", err) }
		return validateImplementationPlan(value)
	case TypeChangeProposal:
		var value ChangeProposalPayload
		if err := decodeStrict(payload, &value); err != nil { return fmt.Errorf("change-proposal payload: %w", err) }
		return validateChangeProposal(value)
	case TypeCodePatchReference:
		var value CodePatchReferencePayload
		if err := decodeStrict(payload, &value); err != nil { return fmt.Errorf("code-patch-reference payload: %w", err) }
		return validateCodePatchReference(value)
	case TypeTestPlan:
		var value TestPlanPayload
		if err := decodeStrict(payload, &value); err != nil { return fmt.Errorf("test-plan payload: %w", err) }
		return validateTestPlan(value)
	case TypeReviewReport:
		var value ReviewReportPayload
		if err := decodeStrict(payload, &value); err != nil { return fmt.Errorf("review-report payload: %w", err) }
		return validateReviewReport(value)
	case TypeRiskAssessment:
		var value RiskAssessmentPayload
		if err := decodeStrict(payload, &value); err != nil { return fmt.Errorf("risk-assessment payload: %w", err) }
		return validateRiskAssessment(value)
	case TypeQuestion:
		var value QuestionPayload
		if err := decodeStrict(payload, &value); err != nil { return fmt.Errorf("question payload: %w", err) }
		return validateQuestion(value)
	case TypeDecision:
		var value DecisionPayload
		if err := decodeStrict(payload, &value); err != nil { return fmt.Errorf("decision payload: %w", err) }
		return validateDecision(value)
	case TypeHandoff:
		if !payloadHasSchemaVersion(payload) {
			return nil // backward compatibility for artifacts persisted before lifecycle payload v1.
		}
		var value HandoffPayload
		if err := decodeStrict(payload, &value); err != nil { return fmt.Errorf("handoff payload: %w", err) }
		return validateHandoff(value)
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
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values are not allowed")
		}
		return fmt.Errorf("invalid trailing JSON: %w", err)
	}
	return nil
}

func payloadHasSchemaVersion(data []byte) bool {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(data, &envelope); err != nil {
		return false
	}
	_, ok := envelope["schema_version"]
	return ok
}

func validateLifecycleVersion(version int) error {
	if version != LifecyclePayloadSchemaVersion {
		return fmt.Errorf("unsupported lifecycle payload schema version: %d", version)
	}
	return nil
}

func validateArchitectureDecision(value ArchitectureDecisionPayload) error {
	if err := requiredBounded("decision", value.Decision, 4000); err != nil { return err }
	if err := requiredBounded("rationale", value.Rationale, 8000); err != nil { return err }
	if err := validateStringList("constraints", value.Constraints, 50, 2000); err != nil { return err }
	if err := validateStringList("interfaces", value.Interfaces, 50, 2000); err != nil { return err }
	return validateStringList("unknowns", value.Unknowns, 50, 2000)
}

func validateAPIContract(value APIContractPayload) error {
	if err := requiredBounded("version", value.Version, 100); err != nil { return err }
	protocol := strings.ToLower(strings.TrimSpace(value.Protocol))
	switch protocol { case "http", "https", "rest": default: return fmt.Errorf("protocol must be http, https, or rest") }
	if len(value.Operations) == 0 { return errors.New("operations must not be empty") }
	if len(value.Operations) > 200 { return errors.New("operations exceeds 200 entries") }
	seen := map[string]bool{}
	for index, operation := range value.Operations {
		if err := requiredBounded(fmt.Sprintf("operations[%d].id", index), operation.ID, 200); err != nil { return err }
		if seen[operation.ID] { return fmt.Errorf("duplicate operation id: %s", operation.ID) }
		seen[operation.ID] = true
		method := strings.ToUpper(strings.TrimSpace(operation.Method))
		switch method {
		case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead, http.MethodOptions:
		default: return fmt.Errorf("unsupported HTTP method for operation %s: %s", operation.ID, operation.Method)
		}
		path := strings.TrimSpace(operation.Path)
		if path == "" || !strings.HasPrefix(path, "/") || len(path) > 1000 { return fmt.Errorf("operation %s path must be an absolute API path", operation.ID) }
		if strings.ContainsAny(path, "\r\n") { return fmt.Errorf("operation %s path must be one line", operation.ID) }
		if len(operation.RequestSchema) > 16000 || len(operation.ResponseSchema) > 16000 { return fmt.Errorf("operation %s schema summary exceeds 16 KiB", operation.ID) }
	}
	return nil
}

func validateVerificationReport(value VerificationReportPayload) error {
	switch strings.ToLower(strings.TrimSpace(value.Status)) {
	case "passed", "failed", "partial", "infrastructure":
	default: return errors.New("verification status must be passed, failed, partial, or infrastructure")
	}
	if len(value.Checks) == 0 { return errors.New("verification checks must not be empty") }
	if len(value.Checks) > 500 { return errors.New("verification checks exceeds 500 entries") }
	seen := map[string]bool{}
	for index, check := range value.Checks {
		if err := requiredBounded(fmt.Sprintf("checks[%d].id", index), check.ID, 300); err != nil { return err }
		if seen[check.ID] { return fmt.Errorf("duplicate verification check id: %s", check.ID) }
		seen[check.ID] = true
		switch strings.ToLower(strings.TrimSpace(check.Status)) {
		case "passed", "failed", "skipped", "partial", "infrastructure":
		default: return fmt.Errorf("unsupported status for verification check %s: %s", check.ID, check.Status)
		}
		if err := requiredBounded(fmt.Sprintf("checks[%d].summary", index), check.Summary, 4000); err != nil { return err }
		if len(check.Evidence) > 8000 { return fmt.Errorf("verification check %s evidence exceeds 8000 characters", check.ID) }
	}
	return nil
}

func validateTaskBrief(value TaskBriefPayload) error {
	if err := validateLifecycleVersion(value.SchemaVersion); err != nil { return err }
	if err := requiredBounded("objective", value.Objective, 8000); err != nil { return err }
	if len(value.AcceptanceCriteria) == 0 { return errors.New("acceptance_criteria must not be empty") }
	if err := validateStringList("acceptance_criteria", value.AcceptanceCriteria, 100, 4000); err != nil { return err }
	if err := validateStringList("constraints", value.Constraints, 100, 4000); err != nil { return err }
	return validateStringList("out_of_scope", value.OutOfScope, 100, 4000)
}

func validateResearchNote(value ResearchNotePayload) error {
	if err := validateLifecycleVersion(value.SchemaVersion); err != nil { return err }
	if err := requiredBounded("finding", value.Finding, 12000); err != nil { return err }
	if err := validateStringList("evidence", value.Evidence, 100, 4000); err != nil { return err }
	return validateStringList("unknowns", value.Unknowns, 100, 4000)
}

func validateImplementationPlan(value ImplementationPlanPayload) error {
	if err := validateLifecycleVersion(value.SchemaVersion); err != nil { return err }
	if err := requiredBounded("goal", value.Goal, 8000); err != nil { return err }
	if len(value.Steps) == 0 { return errors.New("steps must not be empty") }
	if len(value.Steps) > 200 { return errors.New("steps exceeds 200 entries") }
	seen := map[string]bool{}
	for i, step := range value.Steps {
		if err := requiredBounded(fmt.Sprintf("steps[%d].id", i), step.ID, 200); err != nil { return err }
		if seen[step.ID] { return fmt.Errorf("duplicate implementation step id: %s", step.ID) }
		seen[step.ID] = true
		if err := requiredBounded(fmt.Sprintf("steps[%d].summary", i), step.Summary, 4000); err != nil { return err }
		if err := validateStringList(fmt.Sprintf("steps[%d].depends_on", i), step.DependsOn, 100, 200); err != nil { return err }
		if err := validateStringList(fmt.Sprintf("steps[%d].files", i), step.Files, 200, 2000); err != nil { return err }
		if len(step.Verification) > 4000 { return fmt.Errorf("steps[%d].verification exceeds 4000 characters", i) }
	}
	for _, step := range value.Steps {
		for _, dependency := range step.DependsOn {
			if !seen[dependency] { return fmt.Errorf("implementation step %s depends on unknown step %s", step.ID, dependency) }
			if dependency == step.ID { return fmt.Errorf("implementation step %s cannot depend on itself", step.ID) }
		}
	}
	if err := validateStringList("risks", value.Risks, 100, 4000); err != nil { return err }
	return validateStringList("open_questions", value.OpenQuestions, 100, 4000)
}

func validateChangeProposal(value ChangeProposalPayload) error {
	if err := validateLifecycleVersion(value.SchemaVersion); err != nil { return err }
	if err := requiredBounded("objective", value.Objective, 8000); err != nil { return err }
	if len(value.Changes) == 0 { return errors.New("changes must not be empty") }
	if err := validateStringList("changes", value.Changes, 200, 4000); err != nil { return err }
	if err := validateStringList("files", value.Files, 500, 2000); err != nil { return err }
	return validateStringList("risks", value.Risks, 100, 4000)
}

func validateCodePatchReference(value CodePatchReferencePayload) error {
	if err := validateLifecycleVersion(value.SchemaVersion); err != nil { return err }
	if err := requiredBounded("reference", value.Reference, 4000); err != nil { return err }
	if len(value.Files) == 0 { return errors.New("files must not be empty") }
	if err := validateStringList("files", value.Files, 500, 2000); err != nil { return err }
	if len(value.BaseReference) > 4000 { return errors.New("base_reference exceeds 4000 characters") }
	return nil
}

func validateTestPlan(value TestPlanPayload) error {
	if err := validateLifecycleVersion(value.SchemaVersion); err != nil { return err }
	if err := requiredBounded("strategy", value.Strategy, 8000); err != nil { return err }
	if len(value.Cases) == 0 { return errors.New("cases must not be empty") }
	if len(value.Cases) > 500 { return errors.New("cases exceeds 500 entries") }
	seen := map[string]bool{}
	for i, testCase := range value.Cases {
		if err := requiredBounded(fmt.Sprintf("cases[%d].id", i), testCase.ID, 300); err != nil { return err }
		if seen[testCase.ID] { return fmt.Errorf("duplicate test case id: %s", testCase.ID) }
		seen[testCase.ID] = true
		if err := requiredBounded(fmt.Sprintf("cases[%d].scope", i), testCase.Scope, 4000); err != nil { return err }
		if err := requiredBounded(fmt.Sprintf("cases[%d].expected", i), testCase.Expected, 4000); err != nil { return err }
	}
	return nil
}

func validateReviewReport(value ReviewReportPayload) error {
	if err := validateLifecycleVersion(value.SchemaVersion); err != nil { return err }
	switch strings.ToLower(strings.TrimSpace(value.Verdict)) {
	case "approved", "changes-requested", "rejected", "blocked":
	default: return errors.New("review verdict must be approved, changes-requested, rejected, or blocked")
	}
	if len(value.Findings) > 500 { return errors.New("findings exceeds 500 entries") }
	seen := map[string]bool{}
	for i, finding := range value.Findings {
		if err := requiredBounded(fmt.Sprintf("findings[%d].id", i), finding.ID, 300); err != nil { return err }
		if seen[finding.ID] { return fmt.Errorf("duplicate review finding id: %s", finding.ID) }
		seen[finding.ID] = true
		if err := validateSeverity(finding.Severity); err != nil { return fmt.Errorf("finding %s: %w", finding.ID, err) }
		if err := requiredBounded(fmt.Sprintf("findings[%d].summary", i), finding.Summary, 4000); err != nil { return err }
		if len(finding.Evidence) > 8000 { return fmt.Errorf("finding %s evidence exceeds 8000 characters", finding.ID) }
	}
	return nil
}

func validateRiskAssessment(value RiskAssessmentPayload) error {
	if err := validateLifecycleVersion(value.SchemaVersion); err != nil { return err }
	if err := validateSeverity(value.Level); err != nil { return fmt.Errorf("risk level: %w", err) }
	if len(value.Risks) > 200 { return errors.New("risks exceeds 200 entries") }
	seen := map[string]bool{}
	for i, risk := range value.Risks {
		if err := requiredBounded(fmt.Sprintf("risks[%d].id", i), risk.ID, 300); err != nil { return err }
		if seen[risk.ID] { return fmt.Errorf("duplicate risk id: %s", risk.ID) }
		seen[risk.ID] = true
		if err := validateSeverity(risk.Severity); err != nil { return fmt.Errorf("risk %s: %w", risk.ID, err) }
		if err := requiredBounded(fmt.Sprintf("risks[%d].summary", i), risk.Summary, 4000); err != nil { return err }
		if len(risk.Mitigation) > 8000 { return fmt.Errorf("risk %s mitigation exceeds 8000 characters", risk.ID) }
	}
	return nil
}

func validateQuestion(value QuestionPayload) error {
	if err := validateLifecycleVersion(value.SchemaVersion); err != nil { return err }
	if err := requiredBounded("question", value.Question, 8000); err != nil { return err }
	if len(value.Context) > 12000 { return errors.New("context exceeds 12000 characters") }
	return validateStringList("options", value.Options, 50, 4000)
}

func validateDecision(value DecisionPayload) error {
	if err := validateLifecycleVersion(value.SchemaVersion); err != nil { return err }
	if err := requiredBounded("decision", value.Decision, 8000); err != nil { return err }
	if err := requiredBounded("rationale", value.Rationale, 12000); err != nil { return err }
	return validateStringList("consequences", value.Consequences, 100, 4000)
}

func validateHandoff(value HandoffPayload) error {
	if err := validateLifecycleVersion(value.SchemaVersion); err != nil { return err }
	if err := requiredBounded("task_id", value.TaskID, 300); err != nil { return err }
	if err := requiredBounded("agent_id", value.AgentID, 300); err != nil { return err }
	if err := requiredBounded("summary", value.Summary, 8000); err != nil { return err }
	if err := validateArtifactRefList("artifact_refs", value.ArtifactRefs); err != nil { return err }
	return validateMessageRefList("message_refs", value.MessageRefs)
}

func validateSeverity(value string) error {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "info", "low", "medium", "high", "critical":
		return nil
	default:
		return errors.New("severity must be info, low, medium, high, or critical")
	}
}

func validateArtifactRefList(name string, values []string) error {
	if len(values) > 500 { return fmt.Errorf("%s exceeds 500 entries", name) }
	for _, value := range values {
		if !safeID(strings.TrimSpace(value), "artifact-") { return fmt.Errorf("%s contains invalid artifact id: %q", name, value) }
	}
	return nil
}

func validateMessageRefList(name string, values []string) error {
	if len(values) > 500 { return fmt.Errorf("%s exceeds 500 entries", name) }
	for _, value := range values {
		value = strings.TrimSpace(value)
		if !strings.HasPrefix(value, "message-") || len(strings.TrimPrefix(value, "message-")) != 32 {
			return fmt.Errorf("%s contains invalid message id: %q", name, value)
		}
		for _, r := range strings.TrimPrefix(value, "message-") {
			if !(r >= 'a' && r <= 'f' || r >= '0' && r <= '9') {
				return fmt.Errorf("%s contains invalid message id: %q", name, value)
			}
		}
	}
	return nil
}

func requiredBounded(name, value string, max int) error {
	value = strings.TrimSpace(value)
	if value == "" { return fmt.Errorf("%s is required", name) }
	if len(value) > max { return fmt.Errorf("%s exceeds %d characters", name, max) }
	return nil
}

func validateStringList(name string, values []string, maxItems, maxLength int) error {
	if len(values) > maxItems { return fmt.Errorf("%s exceeds %d entries", name, maxItems) }
	normalized := append([]string(nil), values...)
	sort.Strings(normalized)
	for index, value := range normalized {
		value = strings.TrimSpace(value)
		if value == "" { return fmt.Errorf("%s contains an empty entry", name) }
		if len(value) > maxLength { return fmt.Errorf("%s entry %d exceeds %d characters", name, index, maxLength) }
	}
	return nil
}
