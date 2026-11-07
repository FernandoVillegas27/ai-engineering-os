package main

import (
	"testing"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/memory"
)

func TestMemoryGovernanceCLIListDeleteSupersede(t *testing.T) {
	root := t.TempDir()
	t.Setenv("AI_ENGINEERING_OS_MEMORY_DIR", root)

	store, err := memory.NewGovernanceStore(root)
	if err != nil { t.Fatal(err) }
	record, err := store.Save(memory.GovernedSaveRequest{
		SaveRequest: memory.SaveRequest{
			Project: "project-a",
			Title: "Architecture",
			Content: "Use SQLite.",
			Type: memory.TypeArchitecture,
			TopicKey: "architecture/db",
		},
		Retention: 24 * time.Hour,
	}, time.Now().UTC())
	if err != nil { t.Fatal(err) }

	if err := runMemoryCommand([]string{"list", "--project", "project-a", "--json"}); err != nil {
		t.Fatal(err)
	}
	if err := runMemoryCommand([]string{
		"supersede", record.ID,
		"--project", "project-a",
		"--content", "Use PostgreSQL.",
		"--type", string(memory.TypeArchitecture),
		"--retention", "24h",
		"--json",
	}); err != nil {
		t.Fatal(err)
	}

	active, err := store.List("project-a", false, time.Now().UTC())
	if err != nil { t.Fatal(err) }
	if len(active) != 1 || active[0].Content != "Use PostgreSQL." {
		t.Fatalf("active=%#v", active)
	}

	if err := runMemoryCommand([]string{
		"delete", active[0].ID,
		"--project", "project-a",
		"--reason", "obsolete",
		"--json",
	}); err != nil {
		t.Fatal(err)
	}
	active, err = store.List("project-a", false, time.Now().UTC())
	if err != nil { t.Fatal(err) }
	if len(active) != 0 {
		t.Fatalf("deleted memory still active: %#v", active)
	}
	if err := runMemoryCommand([]string{"list", "--project", "project-a", "--all", "--json"}); err != nil {
		t.Fatal(err)
	}
}

func TestMemoryGovernanceCLIRequiresProjectAndValidRetention(t *testing.T) {
	if err := runMemoryCommand([]string{"list"}); err == nil {
		t.Fatal("expected project requirement")
	}
	if err := runMemoryCommand([]string{
		"supersede", "memory-00000000000000000000000000000000",
		"--project", "p",
		"--content", "new",
		"--retention", "invalid",
	}); err == nil {
		t.Fatal("expected invalid retention")
	}
}
