package memory

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGovernanceRedactsSecretsAndPreservesProvenance(t *testing.T) {
	store, err := NewGovernanceStore(t.TempDir())
	if err != nil { t.Fatal(err) }
	now := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	record, err := store.Save(GovernedSaveRequest{
		SaveRequest: SaveRequest{
			Project: "project-a",
			Title: "Deploy token=super-secret-value",
			Content: "Authorization: Bearer abc.def.ghi\napi_key=1234567890abcdef",
			Type: TypeConfig,
			TopicKey: "deploy/config",
		},
		Provider: "engram",
		Provenance: Provenance{
			RunID: "run-1", ArtifactID: "artifact-1", TaskID: "task-1",
			ProducerAgent: "architect", Source: "artifact",
		},
		Retention: 24 * time.Hour,
	}, now)
	if err != nil { t.Fatal(err) }
	if strings.Contains(record.Title, "super-secret-value") ||
		strings.Contains(record.Content, "abc.def.ghi") ||
		strings.Contains(record.Content, "1234567890abcdef") {
		t.Fatalf("secret leaked in record: %#v", record)
	}
	if !strings.Contains(record.Title, "[REDACTED]") || !strings.Contains(record.Content, "[REDACTED]") {
		t.Fatalf("redaction markers missing: %#v", record)
	}
	if record.Provenance.RunID != "run-1" || record.Provenance.Source != "artifact" {
		t.Fatalf("provenance=%#v", record.Provenance)
	}
	if record.ExpiresAt == nil || !record.ExpiresAt.Equal(now.Add(24*time.Hour)) {
		t.Fatalf("expires=%v", record.ExpiresAt)
	}
}

func TestGovernanceDeleteAndSupersedeRemoveOldMemoryFromRecall(t *testing.T) {
	store, _ := NewGovernanceStore(t.TempDir())
	now := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	first, err := store.Save(GovernedSaveRequest{
		SaveRequest: SaveRequest{
			Project: "project-a", Title: "Database choice", Content: "Use SQLite for local state.",
			Type: TypeDecision, TopicKey: "db-choice",
		},
		Retention: 365 * 24 * time.Hour,
	}, now)
	if err != nil { t.Fatal(err) }

	old, next, err := store.Supersede("project-a", first.ID, GovernedSaveRequest{
		SaveRequest: SaveRequest{
			Title: "Database choice", Content: "Use PostgreSQL for shared production state.",
			Type: TypeDecision,
		},
		Provenance: Provenance{Source: "manual-supersede"},
		Retention: 365 * 24 * time.Hour,
	}, now.Add(time.Hour))
	if err != nil { t.Fatal(err) }
	if old.Status != RecordSuperseded || old.SupersededBy != next.ID {
		t.Fatalf("old=%#v next=%#v", old, next)
	}
	recalled, err := store.Recall("project-a", "database production", 5, now.Add(2*time.Hour))
	if err != nil { t.Fatal(err) }
	if len(recalled) != 1 || recalled[0].ID != next.ID || strings.Contains(recalled[0].Content, "SQLite") {
		t.Fatalf("recall=%#v", recalled)
	}

	deleted, err := store.Delete("project-a", next.ID, "obsolete", now.Add(3*time.Hour))
	if err != nil { t.Fatal(err) }
	if deleted.Status != RecordDeleted || deleted.DeletedAt == nil {
		t.Fatalf("deleted=%#v", deleted)
	}
	recalled, err = store.Recall("project-a", "database", 5, now.Add(4*time.Hour))
	if err != nil { t.Fatal(err) }
	if len(recalled) != 0 {
		t.Fatalf("deleted memory recalled: %#v", recalled)
	}
	all, err := store.List("project-a", true, now.Add(4*time.Hour))
	if err != nil { t.Fatal(err) }
	if len(all) != 2 {
		t.Fatalf("all=%#v", all)
	}
}

func TestGovernanceExpiryAndProjectBoundaries(t *testing.T) {
	store, _ := NewGovernanceStore(t.TempDir())
	now := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	record, err := store.Save(GovernedSaveRequest{
		SaveRequest: SaveRequest{
			Project: "project-a", Title: "Temporary finding", Content: "Temporary dependency workaround.",
			Type: TypeDiscovery,
		},
		Retention: time.Hour,
	}, now)
	if err != nil { t.Fatal(err) }
	if values, err := store.Recall("project-a", "workaround", 5, now.Add(30*time.Minute)); err != nil || len(values) != 1 {
		t.Fatalf("before expiry values=%#v err=%v", values, err)
	}
	if values, err := store.Recall("project-a", "workaround", 5, now.Add(2*time.Hour)); err != nil || len(values) != 0 {
		t.Fatalf("after expiry values=%#v err=%v", values, err)
	}
	if _, err := store.Load("project-b", record.ID); err == nil {
		t.Fatal("expected cross-project load rejection")
	}
	if values, err := store.List("project-b", true, now); err != nil || len(values) != 0 {
		t.Fatalf("cross-project list values=%#v err=%v", values, err)
	}
}

func TestGovernanceDetectsTamperedContent(t *testing.T) {
	root := t.TempDir()
	store, _ := NewGovernanceStore(root)
	record, err := store.Save(GovernedSaveRequest{
		SaveRequest: SaveRequest{
			Project: "project-a", Title: "Decision", Content: "Original content", Type: TypeDecision,
		},
	}, time.Now().UTC())
	if err != nil { t.Fatal(err) }
	path := filepath.Join(root, projectDirectoryName("project-a"), record.ID+".json")
	data, err := os.ReadFile(path)
	if err != nil { t.Fatal(err) }
	data = []byte(strings.Replace(string(data), "Original content", "Tampered content", 1))
	if err := os.WriteFile(path, data, 0o600); err != nil { t.Fatal(err) }
	if _, err := store.Load("project-a", record.ID); err == nil || !strings.Contains(err.Error(), "digest mismatch") {
		t.Fatalf("err=%v", err)
	}
}

func TestGovernedProviderUsesLocalLedgerAsRecallAuthority(t *testing.T) {
	store, _ := NewGovernanceStore(t.TempDir())
	mirror := healthyFakeMemory()
	mirror.recall = RecallResult{Context: "UNMANAGED PROVIDER MEMORY"}
	now := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	provider := GovernedProjectMemory{
		Store: store,
		Mirror: mirror,
		Now: func() time.Time { return now },
	}
	result, err := provider.Save(context.Background(), SaveRequest{
		Project: "project-a", Title: "Architecture", Content: "Use event log as source of truth.",
		Type: TypeArchitecture, TopicKey: "architecture/source-of-truth",
		Provenance: Provenance{RunID: "run-1", Source: "artifact"},
		Retention: 365 * 24 * time.Hour,
	})
	if err != nil { t.Fatal(err) }
	if !result.Saved || len(mirror.saves) != 1 {
		t.Fatalf("result=%#v saves=%#v", result, mirror.saves)
	}
	if strings.Contains(mirror.saves[0].Content, "secret") {
		t.Fatal("unexpected secret")
	}

	recalled, err := provider.Recall(context.Background(), RecallRequest{
		Project: "project-a", Query: "event source", Limit: 5,
	})
	if err != nil { t.Fatal(err) }
	if strings.Contains(recalled.Context, "UNMANAGED PROVIDER MEMORY") {
		t.Fatalf("provider recall bypassed governance: %q", recalled.Context)
	}
	if !strings.Contains(recalled.Context, "source of truth") || !strings.Contains(recalled.Context, "provenance:") {
		t.Fatalf("recall=%q", recalled.Context)
	}
}

func TestGovernedProviderMirrorFailureDoesNotLoseGovernedSave(t *testing.T) {
	store, _ := NewGovernanceStore(t.TempDir())
	mirror := healthyFakeMemory()
	mirror.saveErrFor["topic"] = os.ErrPermission
	provider := GovernedProjectMemory{Store: store, Mirror: mirror}

	result, err := provider.Save(context.Background(), SaveRequest{
		Project: "project-a", Title: "Decision", Content: "Keep local ledger authoritative.",
		Type: TypeDecision, TopicKey: "topic",
	})
	if err != nil { t.Fatal(err) }
	if !result.Saved || !strings.Contains(result.Summary, "mirror warning") {
		t.Fatalf("result=%#v", result)
	}
	values, err := store.List("project-a", false, time.Now().UTC())
	if err != nil || len(values) != 1 {
		t.Fatalf("values=%#v err=%v", values, err)
	}
}


func TestGovernanceStableTopicConvergesAndSupersedes(t *testing.T) {
	store, _ := NewGovernanceStore(t.TempDir())
	mirror := healthyFakeMemory()
	now := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	provider := GovernedProjectMemory{Store: store, Mirror: mirror, Now: func() time.Time { return now }}

	first, err := provider.Save(context.Background(), SaveRequest{
		Project: "project-a", Title: "Architecture", Content: "Use SQLite.",
		Type: TypeArchitecture, TopicKey: "architecture/db",
	})
	if err != nil { t.Fatal(err) }
	second, err := provider.Save(context.Background(), SaveRequest{
		Project: "project-a", Title: "Architecture", Content: "Use SQLite.",
		Type: TypeArchitecture, TopicKey: "architecture/db",
	})
	if err != nil { t.Fatal(err) }
	if !strings.Contains(second.Summary, "already current") {
		t.Fatalf("second=%#v", second)
	}
	if len(mirror.saves) != 1 {
		t.Fatalf("identical topic mirrored more than once: %d", len(mirror.saves))
	}

	now = now.Add(time.Hour)
	third, err := provider.Save(context.Background(), SaveRequest{
		Project: "project-a", Title: "Architecture", Content: "Use PostgreSQL.",
		Type: TypeArchitecture, TopicKey: "architecture/db",
	})
	if err != nil { t.Fatal(err) }
	if first.Summary == third.Summary {
		t.Fatalf("replacement did not create new governed record")
	}
	active, err := store.List("project-a", false, now)
	if err != nil { t.Fatal(err) }
	if len(active) != 1 || active[0].Content != "Use PostgreSQL." {
		t.Fatalf("active=%#v", active)
	}
	all, err := store.List("project-a", true, now)
	if err != nil { t.Fatal(err) }
	if len(all) != 2 {
		t.Fatalf("all=%#v", all)
	}
	var superseded bool
	for _, record := range all {
		if record.Status == RecordSuperseded {
			superseded = true
		}
	}
	if !superseded {
		t.Fatalf("expected superseded historical record: %#v", all)
	}
}

func TestGovernanceRejectsSymlinkedProjectDirectory(t *testing.T) {
	root := t.TempDir()
	store, _ := NewGovernanceStore(root)
	outside := t.TempDir()
	projectDir := filepath.Join(root, projectDirectoryName("project-a"))
	if err := os.Symlink(outside, projectDir); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}
	_, err := store.Save(GovernedSaveRequest{
		SaveRequest: SaveRequest{
			Project: "project-a", Title: "Decision", Content: "Do not escape root.", Type: TypeDecision,
		},
	}, time.Now().UTC())
	if err == nil || !strings.Contains(err.Error(), "regular directory") {
		t.Fatalf("err=%v", err)
	}
	entries, readErr := os.ReadDir(outside)
	if readErr != nil { t.Fatal(readErr) }
	if len(entries) != 0 {
		t.Fatalf("outside directory was modified: %#v", entries)
	}
}
