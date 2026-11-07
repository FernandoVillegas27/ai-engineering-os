package toolapproval

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestApprovalEvidenceIsExactRunTaskToolOperationAndHumanOnly(t *testing.T) {
	now := time.Date(2026, 11, 7, 18, 0, 0, 0, time.UTC)
	evidence, err := New(Request{
		RunID: "run-1", TaskID: "task-1", ToolID: "github",
		Operation: "create-pull-request", Status: StatusGranted, Actor: "human",
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if !Grants(evidence, "run-1", "task-1", "github", "create-pull-request", now) {
		t.Fatal("expected exact approval grant")
	}
	for _, tc := range []struct {
		runID, taskID, toolID, operation string
	}{
		{"run-2", "task-1", "github", "create-pull-request"},
		{"run-1", "task-2", "github", "create-pull-request"},
		{"run-1", "task-1", "playwright", "create-pull-request"},
		{"run-1", "task-1", "github", "create-commit"},
	} {
		if Grants(evidence, tc.runID, tc.taskID, tc.toolID, tc.operation, now) {
			t.Fatalf("approval leaked to %#v", tc)
		}
	}
}

func TestApprovalRejectsModelActorAndExpiredEvidence(t *testing.T) {
	now := time.Date(2026, 11, 7, 18, 0, 0, 0, time.UTC)
	if _, err := New(Request{
		RunID: "run-1", TaskID: "task-1", ToolID: "github",
		Operation: "create-commit", Status: StatusGranted, Actor: "model",
	}, now); err == nil {
		t.Fatal("expected non-human actor rejection")
	}
	expires := now.Add(time.Minute)
	evidence, err := New(Request{
		RunID: "run-1", TaskID: "task-1", ToolID: "github",
		Operation: "create-commit", Status: StatusGranted, Actor: "human", ExpiresAt: &expires,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if Grants(evidence, "run-1", "task-1", "github", "create-commit", expires.Add(time.Nanosecond)) {
		t.Fatal("expired approval must fail closed")
	}
}

func TestStoreIsAppendOnlyAndRejectsSymlinkRunDirectory(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	evidence, err := New(Request{
		RunID: "run-1", TaskID: "task-1", ToolID: "playwright",
		Operation: "navigate-browser", Status: StatusRejected, Actor: "human",
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(evidence); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(evidence); err == nil {
		t.Fatal("immutable approval evidence must not be overwritten")
	}
	values, err := store.List("run-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 1 || values[0].ID != evidence.ID {
		t.Fatalf("values=%#v", values)
	}

	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "run-link")
	if err := os.Symlink(target, link); err != nil {
		if os.IsPermission(err) {
			t.Skip("symlink creation not permitted")
		}
		t.Fatal(err)
	}
	if _, err := store.List("run-link"); err == nil {
		t.Fatal("expected symlink run directory rejection")
	}
}


func TestApprovalRequestDigestBindsExactMutationPayload(t *testing.T) {
	now := time.Date(2026, 11, 7, 20, 0, 0, 0, time.UTC)
	digest := DigestRequest([]byte(`{"base":"master","head":"feature-a","title":"Add feature"}`))
	evidence, err := New(Request{
		RunID: "run-1", TaskID: "task-1", ToolID: "github",
		Operation: "create-pull-request", RequestDigest: digest,
		Status: StatusGranted, Actor: "human",
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if !GrantsRequest(evidence, "run-1", "task-1", "github", "create-pull-request", digest, now) {
		t.Fatal("expected exact request digest grant")
	}
	changed := DigestRequest([]byte(`{"base":"master","head":"feature-b","title":"Add feature"}`))
	if GrantsRequest(evidence, "run-1", "task-1", "github", "create-pull-request", changed, now) {
		t.Fatal("approval must not authorize changed mutation payload")
	}
	if GrantsRequest(evidence, "run-1", "task-1", "github", "create-pull-request", "", now) {
		t.Fatal("request-scoped grant must require a digest")
	}
}

func TestApprovalRequestDigestValidationAndLegacyOperationGrant(t *testing.T) {
	now := time.Now().UTC()
	if _, err := New(Request{
		RunID: "run-1", TaskID: "task-1", ToolID: "github",
		Operation: "create-commit", RequestDigest: "not-a-digest",
		Status: StatusGranted, Actor: "human",
	}, now); err == nil {
		t.Fatal("expected invalid digest rejection")
	}
	legacy, err := New(Request{
		RunID: "run-1", TaskID: "task-1", ToolID: "github",
		Operation: "create-commit", Status: StatusGranted, Actor: "human",
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if !Grants(legacy, "run-1", "task-1", "github", "create-commit", now) {
		t.Fatal("legacy operation-scoped grant should remain readable")
	}
	if GrantsRequest(legacy, "run-1", "task-1", "github", "create-commit", DigestRequest([]byte("payload")), now) {
		t.Fatal("legacy grant must not authorize a concrete mutation request")
	}
}
