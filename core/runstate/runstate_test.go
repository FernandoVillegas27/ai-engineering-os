package runstate

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
)

func testRun(t *testing.T) Run {
	t.Helper()
	now := time.Date(2026, 11, 4, 7, 0, 0, 0, time.UTC)
	run, err := New(NewRunRequest{
		Task:         "Implement feature",
		ProjectID:    "project-a",
		ProjectRoot:  "/not/used/by/store",
		Capabilities: []string{"automated-testing", "backend-development", "backend-development"},
		Runtime: RuntimeSelection{
			Provider:    "cursor",
			ModelPolicy: "auto",
		},
		WorkUnits: []execution.WorkUnit{{
			ID:           "work-01-backend",
			Task:         "Implement feature",
			AgentID:      "backend-engineer",
			Capabilities: []string{"backend-development"},
			Skills:       []string{"go"},
			Tools:        []string{"filesystem"},
			Permissions: execution.PermissionEnvelope{
				SourceCode: "read",
				Tests:      "read",
				GitCommit:  "deny",
				GitPush:    "deny",
				Secrets:    "deny",
				Production: "deny",
			},
		}},
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	return run
}

func TestNewRunCreatesStableShape(t *testing.T) {
	run := testRun(t)
	if run.SchemaVersion != SchemaVersion || run.Status != StatusPlanned {
		t.Fatalf("run = %#v", run)
	}
	if run.ID == "" || run.TaskID == "" || run.ID == run.TaskID {
		t.Fatalf("ids: run=%q task=%q", run.ID, run.TaskID)
	}
	if len(run.Capabilities) != 2 || run.Capabilities[0] != "automated-testing" || run.Capabilities[1] != "backend-development" {
		t.Fatalf("capabilities = %#v", run.Capabilities)
	}
	if len(run.WorkUnits) != 1 || run.WorkUnits[0].Status != WorkUnitQueued {
		t.Fatalf("work units = %#v", run.WorkUnits)
	}
}

func TestRunTransitionsAreBounded(t *testing.T) {
	run := testRun(t)
	now := run.UpdatedAt

	if err := Transition(&run, StatusRunning, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := Transition(&run, StatusPaused, now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := Transition(&run, StatusRunning, now.Add(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := Transition(&run, StatusCompleted, now.Add(4*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if run.FinishedAt == nil || run.Status != StatusCompleted {
		t.Fatalf("run = %#v", run)
	}
	if err := Transition(&run, StatusRunning, now.Add(5*time.Minute)); err == nil {
		t.Fatal("terminal run must not resume")
	}
}

func TestValidateRejectsUnknownDependency(t *testing.T) {
	run := testRun(t)
	run.WorkUnits[0].DependsOn = []string{"missing"}
	if err := Validate(run); err == nil {
		t.Fatal("expected unknown dependency error")
	}
}

func TestStoreRoundTripAndUpdate(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	run := testRun(t)
	if err := store.Save(run); err != nil {
		t.Fatal(err)
	}

	loaded, err := store.Load(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ID != run.ID || loaded.Task != run.Task || len(loaded.WorkUnits) != 1 {
		t.Fatalf("loaded = %#v", loaded)
	}

	if err := Transition(&run, StatusRunning, run.UpdatedAt.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(run); err != nil {
		t.Fatal(err)
	}
	loaded, err = store.Load(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != StatusRunning {
		t.Fatalf("loaded status = %q", loaded.Status)
	}

	if _, err := os.Stat(filepath.Join(root, run.ID+".previous")); !os.IsNotExist(err) {
		t.Fatalf("previous file leaked, err=%v", err)
	}
	matches, err := filepath.Glob(filepath.Join(root, ".runstate-*.tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("temp files leaked: %#v", matches)
	}
}

func TestStoreRejectsTraversalAndSymlinkEntry(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load("../outside"); err == nil {
		t.Fatal("expected unsafe id rejection")
	}

	if err := os.WriteFile(filepath.Join(root, "target.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "target.json"), filepath.Join(root, "run-link.json")); err != nil {
		if os.IsPermission(err) {
			t.Skip("symlink creation is not permitted on this host")
		}
		t.Fatal(err)
	}
	if _, err := store.Load("run-link"); err == nil {
		t.Fatal("expected symlink entry rejection")
	}
}

func TestStoreRejectsUnknownFieldsAndSchemaVersion(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	run := testRun(t)
	if err := store.Save(run); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(root, run.ID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data = append(data[:len(data)-2], []byte(",\n  \"unexpected\": true\n}\n")...)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(run.ID); err == nil {
		t.Fatal("expected unknown field rejection")
	}

	run.SchemaVersion = 999
	if err := store.Save(run); err == nil {
		t.Fatal("expected unsupported schema rejection")
	}
}

func TestStoreListNewestFirst(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	first := testRun(t)
	second := testRun(t)
	second.UpdatedAt = first.UpdatedAt.Add(time.Hour)
	second.CreatedAt = first.CreatedAt

	if err := store.Save(first); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(second); err != nil {
		t.Fatal(err)
	}
	runs, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 || runs[0].ID != second.ID {
		t.Fatalf("runs = %#v", runs)
	}
}
