package artifact

import (
	"strings"
	"testing"
	"time"
)

func TestLifecyclePayloadContractsAcceptValidVersionOnePayloads(t *testing.T) {
	cases := []struct {
		kind    Type
		payload any
	}{
		{TypeTaskBrief, TaskBriefPayload{
			SchemaVersion: LifecyclePayloadSchemaVersion,
			Objective: "Ship typed lifecycle contracts.",
			AcceptanceCriteria: []string{"Unknown fields fail closed", "Contracts are provider-independent"},
		}},
		{TypeResearchNote, ResearchNotePayload{
			SchemaVersion: LifecyclePayloadSchemaVersion,
			Finding: "Legacy handoffs need backward-compatible reads.",
			Evidence: []string{"persisted artifact fixtures"},
		}},
		{TypeImplementationPlan, ImplementationPlanPayload{
			SchemaVersion: LifecyclePayloadSchemaVersion,
			Goal: "Introduce typed payloads.",
			Steps: []ImplementationStep{
				{ID: "schema", Summary: "Define contracts"},
				{ID: "producer", Summary: "Wire producers", DependsOn: []string{"schema"}},
			},
		}},
		{TypeChangeProposal, ChangeProposalPayload{
			SchemaVersion: LifecyclePayloadSchemaVersion,
			Objective: "Replace arbitrary lifecycle JSON.",
			Changes: []string{"Add versioned payloads"},
		}},
		{TypeCodePatchReference, CodePatchReferencePayload{
			SchemaVersion: LifecyclePayloadSchemaVersion,
			Reference: "commit:abc123",
			Files: []string{"core/artifact/typed_payload.go"},
		}},
		{TypeTestPlan, TestPlanPayload{
			SchemaVersion: LifecyclePayloadSchemaVersion,
			Strategy: "Validate contracts at creation and load.",
			Cases: []TestCase{{ID: "valid", Scope: "typed payload", Expected: "accepted"}},
		}},
		{TypeReviewReport, ReviewReportPayload{
			SchemaVersion: LifecyclePayloadSchemaVersion,
			Verdict: "approved",
			Findings: []ReviewFinding{{ID: "r1", Severity: "low", Summary: "Minor documentation gap"}},
		}},
		{TypeRiskAssessment, RiskAssessmentPayload{
			SchemaVersion: LifecyclePayloadSchemaVersion,
			Level: "medium",
			Risks: []RiskItem{{ID: "compat", Severity: "medium", Summary: "Legacy handoff records exist", Mitigation: "Read legacy, write v1"}},
		}},
		{TypeQuestion, QuestionPayload{
			SchemaVersion: LifecyclePayloadSchemaVersion,
			Question: "Which contract owns this field?",
			Options: []string{"artifact", "message"},
		}},
		{TypeDecision, DecisionPayload{
			SchemaVersion: LifecyclePayloadSchemaVersion,
			Decision: "Keep provider-independent contracts.",
			Rationale: "Roles must not depend on runtime vendors.",
		}},
		{TypeHandoff, HandoffPayload{
			SchemaVersion: LifecyclePayloadSchemaVersion,
			TaskID: "architect",
			AgentID: "architect",
			Summary: "Architecture ready",
		}},
	}

	for _, tc := range cases {
		t.Run(string(tc.kind), func(t *testing.T) {
			_, err := New(NewRequest{
				RunID: "run-lifecycle",
				TaskID: "task-lifecycle",
				ProducerAgent: "architect",
				Type: tc.kind,
				Summary: "typed lifecycle artifact",
				Payload: tc.payload,
			}, time.Now().UTC())
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestLifecyclePayloadContractsRejectWrongVersionUnknownFieldsAndBrokenGraphs(t *testing.T) {
	_, err := New(NewRequest{
		RunID: "run", TaskID: "task", ProducerAgent: "architect",
		Type: TypeTaskBrief, Summary: "bad",
		Payload: TaskBriefPayload{
			SchemaVersion: 2,
			Objective: "x",
			AcceptanceCriteria: []string{"y"},
		},
	}, time.Now().UTC())
	if err == nil || !strings.Contains(err.Error(), "unsupported lifecycle payload schema version") {
		t.Fatalf("err=%#v", err)
	}

	_, err = New(NewRequest{
		RunID: "run", TaskID: "task", ProducerAgent: "architect",
		Type: TypeReviewReport, Summary: "bad",
		Payload: map[string]any{
			"schema_version": 1,
			"verdict": "approved",
			"unexpected": true,
		},
	}, time.Now().UTC())
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("err=%#v", err)
	}

	_, err = New(NewRequest{
		RunID: "run", TaskID: "task", ProducerAgent: "architect",
		Type: TypeImplementationPlan, Summary: "bad",
		Payload: ImplementationPlanPayload{
			SchemaVersion: LifecyclePayloadSchemaVersion,
			Goal: "bad graph",
			Steps: []ImplementationStep{
				{ID: "one", Summary: "step", DependsOn: []string{"missing"}},
			},
		},
	}, time.Now().UTC())
	if err == nil || !strings.Contains(err.Error(), "depends on unknown step") {
		t.Fatalf("err=%#v", err)
	}
}

func TestHandoffPayloadKeepsLegacyReadCompatibilityButVersionedWritesAreStrict(t *testing.T) {
	legacy, err := New(NewRequest{
		RunID: "run", TaskID: "task", ProducerAgent: "architect",
		Type: TypeHandoff, Summary: "legacy",
		Payload: map[string]any{"summary": "old persisted shape"},
	}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(legacy); err != nil {
		t.Fatal(err)
	}

	_, err = New(NewRequest{
		RunID: "run", TaskID: "task", ProducerAgent: "architect",
		Type: TypeHandoff, Summary: "bad typed",
		Payload: map[string]any{
			"schema_version": 1,
			"task_id": "task",
			"agent_id": "architect",
			"summary": "x",
			"unexpected": true,
		},
	}, time.Now().UTC())
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("err=%#v", err)
	}
}
