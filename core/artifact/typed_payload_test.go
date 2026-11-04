package artifact

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTypedArtifact(t *testing.T, kind Type, payload any) Artifact {
	t.Helper()
	value, err := New(NewRequest{
		RunID:         "run-typed",
		TaskID:        "task-typed",
		ProducerAgent: "architect",
		Type:          kind,
		Summary:       "typed artifact",
		Payload:       payload,
	}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestArchitectureDecisionPayloadValidation(t *testing.T) {
	value := newTypedArtifact(t, TypeArchitectureDecision, ArchitectureDecisionPayload{
		Decision:    "Use typed inter-agent artifacts.",
		Rationale:   "Receivers need stable provider-independent contracts.",
		Constraints: []string{"No provider-specific fields"},
		Interfaces:  []string{"artifact.Store"},
		Unknowns:    []string{"Future schema migration policy"},
	})
	if value.Type != TypeArchitectureDecision {
		t.Fatalf("artifact = %#v", value)
	}

	_, err := New(NewRequest{
		RunID: "run", TaskID: "task", ProducerAgent: "architect",
		Type: TypeArchitectureDecision, Summary: "bad",
		Payload: map[string]any{
			"decision":  "x",
			"rationale": "",
		},
	}, time.Now().UTC())
	if err == nil || !strings.Contains(err.Error(), "rationale is required") {
		t.Fatalf("err = %#v", err)
	}
}

func TestArchitectureDecisionRejectsUnknownPayloadFields(t *testing.T) {
	_, err := New(NewRequest{
		RunID: "run", TaskID: "task", ProducerAgent: "architect",
		Type: TypeArchitectureDecision, Summary: "bad",
		Payload: map[string]any{
			"decision":   "x",
			"rationale":  "y",
			"unexpected": true,
		},
	}, time.Now().UTC())
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("err = %#v", err)
	}
}

func TestAPIContractPayloadValidation(t *testing.T) {
	value := newTypedArtifact(t, TypeAPIContract, APIContractPayload{
		Version:  "v1",
		Protocol: "rest",
		Operations: []APIOperation{
			{
				ID:             "get-user",
				Method:         "GET",
				Path:           "/api/users/{id}",
				ResponseSchema: "User",
			},
			{
				ID:             "update-user",
				Method:         "PATCH",
				Path:           "/api/users/{id}",
				RequestSchema:  "UpdateUser",
				ResponseSchema: "User",
			},
		},
	})
	if value.Type != TypeAPIContract {
		t.Fatalf("artifact = %#v", value)
	}

	cases := []APIContractPayload{
		{Version: "v1", Protocol: "grpc", Operations: []APIOperation{{ID: "x", Method: "GET", Path: "/x"}}},
		{Version: "v1", Protocol: "rest", Operations: nil},
		{Version: "v1", Protocol: "rest", Operations: []APIOperation{{ID: "x", Method: "INVALID", Path: "/x"}}},
		{Version: "v1", Protocol: "rest", Operations: []APIOperation{{ID: "x", Method: "GET", Path: "relative"}}},
		{Version: "v1", Protocol: "rest", Operations: []APIOperation{
			{ID: "dup", Method: "GET", Path: "/a"},
			{ID: "dup", Method: "GET", Path: "/b"},
		}},
	}
	for _, payload := range cases {
		_, err := New(NewRequest{
			RunID: "run", TaskID: "task", ProducerAgent: "backend-engineer",
			Type: TypeAPIContract, Summary: "bad contract", Payload: payload,
		}, time.Now().UTC())
		if err == nil {
			t.Fatalf("expected invalid contract: %#v", payload)
		}
	}
}

func TestVerificationReportPayloadValidation(t *testing.T) {
	value := newTypedArtifact(t, TypeVerificationReport, VerificationReportPayload{
		Status: "failed",
		Checks: []VerificationCheck{
			{ID: "go-test", Status: "failed", Summary: "one regression", Evidence: "pkg#TestNew"},
			{ID: "go-vet", Status: "passed", Summary: "clean"},
		},
	})
	if value.Type != TypeVerificationReport {
		t.Fatalf("artifact = %#v", value)
	}

	_, err := New(NewRequest{
		RunID: "run", TaskID: "task", ProducerAgent: "qa-engineer",
		Type: TypeVerificationReport, Summary: "bad",
		Payload: VerificationReportPayload{
			Status: "maybe",
			Checks:  []VerificationCheck{{ID: "x", Status: "passed", Summary: "ok"}},
		},
	}, time.Now().UTC())
	if err == nil {
		t.Fatal("expected invalid verification status")
	}
}

func TestTypedPayloadValidationAlsoAppliesWhenLoadingPersistedArtifact(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	value := newTypedArtifact(t, TypeArchitectureDecision, ArchitectureDecisionPayload{
		Decision: "D",
		Rationale: "R",
	})
	if err := store.Save(value); err != nil {
		t.Fatal(err)
	}

	value.Payload = json.RawMessage(`{"decision":"D","rationale":"R","unexpected":true}`)
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, value.ID+".json")
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(value.ID); err == nil {
		t.Fatal("expected persisted typed payload validation")
	}
}
