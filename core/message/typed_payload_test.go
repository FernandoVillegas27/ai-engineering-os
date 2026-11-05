package message

import (
	"strings"
	"testing"
	"time"
)

func newTypedMessageForTest(t *testing.T, kind Kind, payload any) Message {
	t.Helper()
	value, err := New(NewRequest{
		RunID: "run-typed-message",
		TaskID: "task-1",
		FromAgent: "architect",
		To: Target{Kind: TargetAgent, ID: "backend-engineer"},
		Kind: kind,
		Payload: payload,
	}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestTypedCoordinationPayloads(t *testing.T) {
	cases := []struct {
		kind Kind
		payload any
	}{
		{KindHandoff, HandoffPayload{SchemaVersion: PayloadSchemaVersion, FromTask: "architect", ToTask: "backend", Summary: "contract ready"}},
		{KindQuestion, QuestionPayload{SchemaVersion: PayloadSchemaVersion, Question: "Which DTO?", Options: []string{"v1", "v2"}}},
		{KindAnswer, AnswerPayload{SchemaVersion: PayloadSchemaVersion, Answer: "Use v2", Rationale: "Current contract"}},
		{KindBlocker, BlockerPayload{SchemaVersion: PayloadSchemaVersion, Reason: "Missing fixture", Needs: "Test data"}},
		{KindDecision, DecisionPayload{SchemaVersion: PayloadSchemaVersion, Decision: "Keep endpoint stable", Rationale: "Compatibility"}},
		{KindStatus, StatusPayload{SchemaVersion: PayloadSchemaVersion, State: "waiting", Detail: "Dependency pending"}},
	}
	for _, tc := range cases {
		t.Run(string(tc.kind), func(t *testing.T) {
			newTypedMessageForTest(t, tc.kind, tc.payload)
		})
	}
}

func TestTypedCoordinationPayloadRejectsUnknownFieldAndUnsupportedVersion(t *testing.T) {
	_, err := New(NewRequest{
		RunID: "run", TaskID: "task", FromAgent: "architect",
		To: Target{Kind: TargetAgent, ID: "backend-engineer"},
		Kind: KindHandoff,
		Payload: map[string]any{
			"schema_version": 1,
			"from_task": "a",
			"to_task": "b",
			"summary": "ready",
			"unexpected": true,
		},
	}, time.Now().UTC())
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("err=%#v", err)
	}

	_, err = New(NewRequest{
		RunID: "run", TaskID: "task", FromAgent: "architect",
		To: Target{Kind: TargetAgent, ID: "backend-engineer"},
		Kind: KindStatus,
		Payload: StatusPayload{SchemaVersion: 99, State: "running"},
	}, time.Now().UTC())
	if err == nil || !strings.Contains(err.Error(), "unsupported message payload schema version") {
		t.Fatalf("err=%#v", err)
	}
}

func TestLegacyUnversionedMessagePayloadRemainsReadable(t *testing.T) {
	value, err := New(NewRequest{
		RunID: "run", TaskID: "task", FromAgent: "architect",
		To: Target{Kind: TargetAgent, ID: "backend-engineer"},
		Kind: KindHandoff,
		Payload: map[string]string{"summary": "legacy"},
	}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(value); err != nil {
		t.Fatal(err)
	}
}
