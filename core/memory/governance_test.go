package memory

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

type governedFakeMemory struct {
	saves   []SaveRequest
	deletes []DeleteRequest
	nextID  int
}

func (f *governedFakeMemory) ID() string { return "fake" }
func (f *governedFakeMemory) Status(context.Context) (Status, error) {
	return Status{Provider:"fake", Available:true, Healthy:true}, nil
}
func (f *governedFakeMemory) Recall(_ context.Context, request RecallRequest) (RecallResult, error) {
	return RecallResult{Provider:"fake", Project:request.Project, Query:request.Query, Context:"ok"}, nil
}
func (f *governedFakeMemory) Save(_ context.Context, request SaveRequest) (SaveResult, error) {
	f.nextID++
	f.saves = append(f.saves, request)
	return SaveResult{Provider:"fake", Project:request.Project, Saved:true, MemoryID:fmtInt(f.nextID)}, nil
}
func (f *governedFakeMemory) Delete(_ context.Context, request DeleteRequest) (DeleteResult, error) {
	f.deletes = append(f.deletes, request)
	return DeleteResult{Provider:"fake", Project:request.Project, MemoryID:request.MemoryID, Deleted:true, Hard:request.Hard}, nil
}

func fmtInt(value int) string {
	const digits = "0123456789"
	if value == 0 { return "0" }
	var out []byte
	for value > 0 {
		out = append([]byte{digits[value%10]}, out...)
		value /= 10
	}
	return string(out)
}

func TestGovernedProviderRedactsTracksSupersedesReviewsAndDeletes(t *testing.T) {
	store, err := NewGovernanceStore(t.TempDir())
	if err != nil { t.Fatal(err) }
	provider := &governedFakeMemory{}
	governed := GovernedProvider{Inner: provider, Store: store}

	first, err := governed.Save(context.Background(), SaveRequest{
		Project:"project-a",
		Title:"API auth",
		Content:"password=supersecret bearer abcdefghijklmnop",
		Type:TypeDecision,
		TopicKey:"decision/api-auth",
		Provenance: Provenance{RunID:"run-1", TaskID:"architect", AgentID:"architect", Source:"artifact"},
	})
	if err != nil { t.Fatal(err) }
	if first.GovernanceID == "" || first.MemoryID != "1" { t.Fatalf("first=%#v", first) }
	if strings.Contains(provider.saves[0].Content, "supersecret") ||
		strings.Contains(provider.saves[0].Content, "abcdefghijklmnop") {
		t.Fatalf("secret reached provider: %q", provider.saves[0].Content)
	}

	second, err := governed.Save(context.Background(), SaveRequest{
		Project:"project-a", Title:"API auth v2", Content:"Use signed sessions",
		Type:TypeDecision, TopicKey:"decision/api-auth",
	})
	if err != nil { t.Fatal(err) }

	records, err := store.Records("project-a")
	if err != nil { t.Fatal(err) }
	if len(records) != 2 ||
		records[0].State != GovernanceSupersededState ||
		records[0].SupersededBy != second.GovernanceID ||
		records[1].Supersedes != first.GovernanceID {
		t.Fatalf("records=%#v", records)
	}

	reviewed, err := governed.Review(context.Background(), "project-a", second.GovernanceID)
	if err != nil { t.Fatal(err) }
	if reviewed.ReviewedAt == nil { t.Fatalf("reviewed=%#v", reviewed) }

	deleted, err := governed.Delete(context.Background(), "project-a", second.GovernanceID, false)
	if err != nil { t.Fatal(err) }
	if deleted.State != GovernanceDeletedState || deleted.DeletedAt == nil {
		t.Fatalf("deleted=%#v", deleted)
	}
	if len(provider.deletes) != 1 || provider.deletes[0].Project != "project-a" ||
		provider.deletes[0].MemoryID != "2" {
		t.Fatalf("deletes=%#v", provider.deletes)
	}
}

func TestGovernanceProjectBoundaryAndTamperFailClosed(t *testing.T) {
	root := t.TempDir()
	store, _ := NewGovernanceStore(root)
	provider := &governedFakeMemory{}
	governed := GovernedProvider{Inner:provider, Store:store}
	saved, err := governed.Save(context.Background(), SaveRequest{
		Project:"alpha", Title:"Decision", Content:"Keep explicit approval",
		Type:TypeDecision, TopicKey:"decision/promotion",
	})
	if err != nil { t.Fatal(err) }

	if _, err := governed.Delete(context.Background(), "beta", saved.GovernanceID, false); !os.IsNotExist(err) {
		t.Fatalf("cross-project delete err=%v", err)
	}

	dir := filepath.Join(root, governanceProjectKey("alpha"))
	entries, err := os.ReadDir(dir)
	if err != nil { t.Fatal(err) }
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) != ".json" { continue }
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil { t.Fatal(err) }
		data = []byte(strings.Replace(string(data), "Keep explicit approval", "Tampered approval rule", 1))
		if err := os.WriteFile(path, data, 0o600); err != nil { t.Fatal(err) }
		break
	}
	if _, err := store.Records("alpha"); err == nil || !strings.Contains(err.Error(), "digest mismatch") {
		t.Fatalf("tamper err=%v", err)
	}
}

func TestGovernedPruneIsExplicitAndRetentionBounded(t *testing.T) {
	store, _ := NewGovernanceStore(t.TempDir())
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }
	provider := &governedFakeMemory{}
	governed := GovernedProvider{Inner:provider, Store:store}
	saved, err := governed.Save(context.Background(), SaveRequest{
		Project:"project", Title:"Old", Content:"Old learning", Type:TypeLearning, TopicKey:"learning/old",
	})
	if err != nil { t.Fatal(err) }
	store.now = func() time.Time { return now.Add(100 * 24 * time.Hour) }

	// Merely listing never applies retention.
	records, err := store.Records("project")
	if err != nil || records[0].State != GovernanceActive { t.Fatalf("records=%#v err=%v", records, err) }

	// Explicit prune is the only destructive retention action.
	deleted, err := governed.Prune(context.Background(), "project", 90*24*time.Hour, false)
	if err != nil { t.Fatal(err) }
	if len(deleted) != 1 || deleted[0].GovernanceID != saved.GovernanceID {
		t.Fatalf("deleted=%#v", deleted)
	}
}

func TestRedactSensitiveCommonSecrets(t *testing.T) {
	value := RedactSensitive("api_key=abcdef123456 password=hunter22 Authorization: Bearer abcdefghijklmnop ghp_abcdefghijklmnopqrstuvwxyz1234 AKIAABCDEFGHIJKLMNOP")
	for _, secret := range []string{"abcdef123456","hunter22","abcdefghijklmnop","ghp_abcdefghijklmnopqrstuvwxyz1234","AKIAABCDEFGHIJKLMNOP"} {
		if strings.Contains(value, secret) { t.Fatalf("secret %q remained in %q", secret, value) }
	}
}
