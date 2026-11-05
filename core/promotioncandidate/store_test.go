package promotioncandidate

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/promotion"
	"github.com/FernandoVillegas27/ai-engineering-os/core/staging"
)

func testRequest() promotion.Request {
	var one [32]byte
	one[0] = 1
	var two [32]byte
	two[0] = 2
	return promotion.Request{
		OriginalRoot:    "/tmp/original",
		StagedWorkspace: "/tmp/staged",
		PreImage: staging.NewPreImage(staging.WorkspaceSnapshot{
			"main.go": two,
			"a.go":    one,
		}),
		Declared: []staging.WorkspaceChange{
			{Path: "main.go", Action: "modify"},
			{Path: "new.go", Action: "create"},
		},
		QualityPassed: true,
		TempParent:    "/tmp",
	}
}

func TestRecordRoundTripReconstructsPromotionRequest(t *testing.T) {
	now := time.Date(2026, 11, 4, 23, 0, 0, 0, time.UTC)
	record, err := New("run-123", testRequest(), now)
	if err != nil {
		t.Fatal(err)
	}
	request, err := record.Request()
	if err != nil {
		t.Fatal(err)
	}
	if request.PreImage.Digest() != testRequest().PreImage.Digest() {
		t.Fatalf("digest = %q", request.PreImage.Digest())
	}
	if request.OriginalRoot != "/tmp/original" || request.StagedWorkspace != "/tmp/staged" {
		t.Fatalf("request = %#v", request)
	}
	if len(request.Declared) != 2 || !request.QualityPassed {
		t.Fatalf("request = %#v", request)
	}
}

func TestStoreRoundTripAndImmutableRunID(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	record, err := New("run-123", testRequest(), time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(record); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load("run-123")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.PreImageDigest != record.PreImageDigest || len(loaded.Declared) != 2 {
		t.Fatalf("loaded = %#v", loaded)
	}
	if err := store.Save(record); err == nil {
		t.Fatal("expected immutable candidate overwrite rejection")
	}
}

func TestLoadRejectsTamperedPreImageDigest(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	record, err := New("run-123", testRequest(), time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	record.PreImageDigest = strings.Repeat("0", 64)
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "run-123.json"), append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load("run-123"); err == nil {
		t.Fatal("expected tampered digest rejection")
	}
}

func TestStoreRejectsUnsafeRunIDAndSymlinkEntry(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load("../outside"); err == nil {
		t.Fatal("expected unsafe run id rejection")
	}

	target := filepath.Join(root, "target.json")
	if err := os.WriteFile(target, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "run-link.json")
	if err := os.Symlink(target, link); err != nil {
		if os.IsPermission(err) {
			t.Skip("symlink creation not permitted")
		}
		t.Fatal(err)
	}
	if _, err := store.Load("run-link"); err == nil {
		t.Fatal("expected symlink candidate rejection")
	}
}

func TestStoredCandidateStillRequiresPromotionApproval(t *testing.T) {
	record, err := New("run-123", testRequest(), time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	request, err := record.Request()
	if err != nil {
		t.Fatal(err)
	}
	result, err := promotion.Apply(context.Background(), promotion.ApplyRequest{
		Request:  request,
		Approved: false,
	})
	if err == nil || result.Applied {
		t.Fatalf("result=%#v err=%#v", result, err)
	}
}
