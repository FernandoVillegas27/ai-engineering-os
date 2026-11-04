package artifact

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testArtifact(t *testing.T) Artifact {
	t.Helper()
	value, err := New(NewRequest{
		RunID:         "run-123",
		TaskID:        "task-456",
		ProducerAgent: "architect",
		Type:          TypeArchitectureDecision,
		Summary:       "Use typed inter-agent messages.",
		Payload: ArchitectureDecisionPayload{
			Decision:  "Messages use a versioned envelope.",
			Rationale: "Provider-independent coordination.",
		},
		Evidence: []EvidenceRef{{Kind: "doc", Value: "docs/MASTER_PLAN.md"}},
	}, time.Date(2026, 11, 4, 7, 30, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestNewArtifactCreatesVersionedImmutableIdentity(t *testing.T) {
	value := testArtifact(t)
	if value.SchemaVersion != SchemaVersion || value.ID == "" {
		t.Fatalf("artifact = %#v", value)
	}
	if value.Type != TypeArchitectureDecision || value.Status != StatusActive {
		t.Fatalf("artifact = %#v", value)
	}
	if !json.Valid(value.Payload) {
		t.Fatalf("invalid payload: %s", value.Payload)
	}
}

func TestValidateRejectsSelfRelationAndInvalidPayload(t *testing.T) {
	value := testArtifact(t)
	value.Related = []string{value.ID}
	if err := Validate(value); err == nil {
		t.Fatal("expected self relation rejection")
	}

	value = testArtifact(t)
	value.Payload = json.RawMessage("{")
	if err := Validate(value); err == nil {
		t.Fatal("expected invalid JSON payload rejection")
	}
}

func TestStoreRoundTripAndRejectsOverwrite(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	value := testArtifact(t)
	if err := store.Save(value); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(value.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ID != value.ID || loaded.Summary != value.Summary || loaded.ProducerAgent != "architect" {
		t.Fatalf("loaded = %#v", loaded)
	}
	if err := store.Save(value); err == nil {
		t.Fatal("immutable artifact overwrite should fail")
	}
}

func TestStoreRejectsTraversalAndSymlink(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load("../outside"); err == nil {
		t.Fatal("expected unsafe artifact id rejection")
	}

	target := filepath.Join(root, "target.json")
	if err := os.WriteFile(target, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "artifact-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.json")
	if err := os.Symlink(target, link); err != nil {
		if os.IsPermission(err) {
			t.Skip("symlink creation not permitted on this host")
		}
		t.Fatal(err)
	}
	if _, err := store.Load("artifact-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"); err == nil {
		t.Fatal("expected symlink artifact rejection")
	}
}

func TestArtifactRelatedIDsAreSortedAndDeduplicated(t *testing.T) {
	one := "artifact-11111111111111111111111111111111"
	two := "artifact-22222222222222222222222222222222"
	value, err := New(NewRequest{
		RunID:         "run-1",
		TaskID:        "task-1",
		ProducerAgent: "backend-engineer",
		Type:          TypeHandoff,
		Summary:       "Hand off API contract to frontend.",
		Payload:       map[string]string{"target": "frontend-engineer"},
		Related:       []string{two, one, two},
	}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if len(value.Related) != 2 || value.Related[0] != one || value.Related[1] != two {
		t.Fatalf("related = %#v", value.Related)
	}
}
