package runstate

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/scheduler"
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
			ToolOperations: map[string][]string{"filesystem": {"read-files"}},
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
	if got := run.WorkUnits[0].ToolOperations["filesystem"]; len(got) != 1 || got[0] != "read-files" {
		t.Fatalf("tool operations = %#v", run.WorkUnits[0].ToolOperations)
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


func TestBindTaskGraphPersistsDependenciesAndStatuses(t *testing.T) {
	now := time.Now().UTC()
	units := []execution.WorkUnit{
		{ID: "architect", AgentID: "architect", Task: "design", Permissions: execution.PermissionEnvelope{SourceCode: "read", Tests: "read", GitCommit: "deny", GitPush: "deny", Secrets: "deny", Production: "deny"}},
		{ID: "backend", AgentID: "backend-engineer", Task: "implement", Permissions: execution.PermissionEnvelope{SourceCode: "read", Tests: "read", GitCommit: "deny", GitPush: "deny", Secrets: "deny", Production: "deny"}},
	}
	run, err := New(NewRunRequest{
		Task: "build feature", ProjectID: "project", ProjectRoot: ".", WorkUnits: units,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	tasks := []scheduler.Task{
		{ID: "architect", AgentID: "architect", Status: scheduler.StatusCompleted},
		{ID: "backend", AgentID: "backend-engineer", DependsOn: []string{"architect"}, Mutating: true, Scopes: []string{"backend"}, Status: scheduler.StatusQueued},
	}
	if err := BindTaskGraph(&run, tasks); err != nil {
		t.Fatal(err)
	}
	if len(run.WorkUnits[1].DependsOn) != 1 || run.WorkUnits[1].DependsOn[0] != "architect" {
		t.Fatalf("backend deps = %#v", run.WorkUnits[1].DependsOn)
	}
	if run.WorkUnits[0].Status != WorkUnitCompleted || run.WorkUnits[1].Status != WorkUnitQueued {
		t.Fatalf("statuses = %#v", run.WorkUnits)
	}
	if run.WorkUnits[0].Mutating || !run.WorkUnits[1].Mutating {
		t.Fatalf("mutating metadata = %#v", run.WorkUnits)
	}
	if len(run.WorkUnits[1].Scopes) != 1 || run.WorkUnits[1].Scopes[0] != "backend" {
		t.Fatalf("backend scopes = %#v", run.WorkUnits[1].Scopes)
	}
}

func TestBindTaskGraphRejectsIncompleteOrMismatchedGraph(t *testing.T) {
	run, err := New(NewRunRequest{
		Task: "x", ProjectID: "project",
		WorkUnits: []execution.WorkUnit{
			{ID: "backend", AgentID: "backend-engineer", Task: "x", Permissions: execution.PermissionEnvelope{SourceCode: "read", Tests: "read", GitCommit: "deny", GitPush: "deny", Secrets: "deny", Production: "deny"}},
		},
	}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := BindTaskGraph(&run, []scheduler.Task{}); err == nil {
		t.Fatal("expected empty graph rejection")
	}
	if err := BindTaskGraph(&run, []scheduler.Task{{ID: "backend", AgentID: "frontend-engineer"}}); err == nil {
		t.Fatal("expected agent mismatch rejection")
	}
}


func TestNewPersistsRetryLineageAndIntent(t *testing.T) {
	run, err := New(NewRunRequest{
		Task:          "retry backend analysis",
		Intent:        "research",
		ParentRunID:   "run-parent",
		RetryOfTaskID: "work-backend",
		ProjectID:     "project",
		Runtime: RuntimeSelection{
			Provider:    "codex",
			ModelPolicy: "auto",
			Model:       "model-x",
		},
		WorkUnits: []execution.WorkUnit{{
			ID: "work-backend", AgentID: "backend-engineer", Task: "retry backend analysis",
			Permissions: execution.PermissionEnvelope{
				SourceCode: "read", Tests: "read", GitCommit: "deny",
				GitPush: "deny", Secrets: "deny", Production: "deny",
			},
		}},
	}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if run.ParentRunID != "run-parent" || run.RetryOfTaskID != "work-backend" || run.Intent != "research" {
		t.Fatalf("run = %#v", run)
	}
	if run.Runtime.Provider != "codex" || run.Runtime.ModelPolicy != "auto" || run.Runtime.Model != "model-x" {
		t.Fatalf("runtime = %#v", run.Runtime)
	}
}

func TestValidateRejectsPartialRetryLineage(t *testing.T) {
	run, err := New(NewRunRequest{
		Task: "x", ProjectID: "project",
		WorkUnits: []execution.WorkUnit{{
			ID: "work-1", AgentID: "researcher", Task: "x",
			Permissions: execution.PermissionEnvelope{
				SourceCode: "read", Tests: "read", GitCommit: "deny",
				GitPush: "deny", Secrets: "deny", Production: "deny",
			},
		}},
	}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	run.ParentRunID = "run-parent"
	if err := Validate(run); err == nil {
		t.Fatal("expected partial retry lineage rejection")
	}
}


func TestBindMutationKindsUsesConfirmedTaskMutationAndRegistryCapability(t *testing.T) {
	units := []execution.WorkUnit{
		{
			ID: "architect", AgentID: "architect", Task: "x",
			Permissions: execution.PermissionEnvelope{SourceCode: "read", Tests: "read", GitCommit: "deny", GitPush: "deny", Secrets: "deny", Production: "deny"},
			Mode: execution.ModeReadOnly,
		},
		{
			ID: "backend", AgentID: "backend-engineer", Task: "x",
			Permissions: execution.PermissionEnvelope{SourceCode: "read", Tests: "read", GitCommit: "deny", GitPush: "deny", Secrets: "deny", Production: "deny"},
			Mutation: execution.MutationCapability{SourceCode: true, Tests: true},
			Mode: execution.ModeReadOnly,
		},
		{
			ID: "qa", AgentID: "qa-engineer", Task: "x",
			Permissions: execution.PermissionEnvelope{SourceCode: "read", Tests: "read", GitCommit: "deny", GitPush: "deny", Secrets: "deny", Production: "deny"},
			Mutation: execution.MutationCapability{Tests: true},
			Mode: execution.ModeReadOnly,
		},
	}
	run, err := New(NewRunRequest{
		Task: "x", Intent: "feature", ProjectID: "project", WorkUnits: units,
	}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := BindTaskGraph(&run, []scheduler.Task{
		{ID: "architect", AgentID: "architect"},
		{ID: "backend", AgentID: "backend-engineer", DependsOn: []string{"architect"}, Mutating: true},
		{ID: "qa", AgentID: "qa-engineer", DependsOn: []string{"backend"}, Mutating: true},
	}); err != nil {
		t.Fatal(err)
	}
	if err := BindMutationKinds(&run, units); err != nil {
		t.Fatal(err)
	}
	kinds := map[string]string{}
	for _, unit := range run.WorkUnits {
		kinds[unit.ID] = unit.MutationKind
	}
	if kinds["architect"] != "" || kinds["backend"] != "source" || kinds["qa"] != "tests-only" {
		t.Fatalf("mutation kinds=%#v", kinds)
	}
}

func TestBindMutationKindsRejectsMutatingUnitWithoutWriteCapability(t *testing.T) {
	unit := execution.WorkUnit{
		ID: "architect", AgentID: "architect", Task: "x",
		Permissions: execution.PermissionEnvelope{SourceCode: "read", Tests: "read", GitCommit: "deny", GitPush: "deny", Secrets: "deny", Production: "deny"},
		Mode: execution.ModeReadOnly,
	}
	run, err := New(NewRunRequest{
		Task: "x", ProjectID: "project", WorkUnits: []execution.WorkUnit{unit},
	}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := BindTaskGraph(&run, []scheduler.Task{{
		ID: "architect", AgentID: "architect", Mutating: true,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := BindMutationKinds(&run, []execution.WorkUnit{unit}); err == nil {
		t.Fatal("expected missing write capability rejection")
	}
}


func TestProjectCodeTrustPersistsAcrossStoreRoundTrip(t *testing.T) {
	now := time.Now().UTC()
	run, err := New(NewRunRequest{
		Task: "trusted mutation",
		ProjectID: "project",
		ProjectRoot: ".",
		ProjectCodeTrusted: true,
		WorkUnits: []execution.WorkUnit{{
			ID: "backend", AgentID: "backend-engineer", Task: "trusted mutation",
			Permissions: execution.PermissionEnvelope{
				SourceCode: "write", Tests: "write", GitCommit: "deny",
				GitPush: "deny", Secrets: "deny", Production: "deny",
			},
		}},
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if !run.ProjectCodeTrusted {
		t.Fatal("trust evidence was not persisted in new run")
	}
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(run); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.ProjectCodeTrusted {
		t.Fatalf("loaded run lost project-code trust: %#v", loaded)
	}
}
