package scheduler

import "testing"

func TestSchedulerRespectsDependenciesAndParallelReadyTasks(t *testing.T) {
	s, err := New([]Task{
		{ID: "architecture", AgentID: "architect"},
		{ID: "backend", AgentID: "backend-engineer", DependsOn: []string{"architecture"}, Mutating: true, Scopes: []string{"backend"}},
		{ID: "frontend", AgentID: "frontend-engineer", DependsOn: []string{"architecture"}, Mutating: true, Scopes: []string{"frontend"}},
		{ID: "qa", AgentID: "qa-engineer", DependsOn: []string{"backend", "frontend"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	first := s.AcquireReady(10)
	if len(first) != 1 || first[0].ID != "architecture" {
		t.Fatalf("first ready = %#v", first)
	}
	if err := s.Complete("architecture"); err != nil {
		t.Fatal(err)
	}

	second := s.AcquireReady(10)
	if len(second) != 2 || second[0].ID != "backend" || second[1].ID != "frontend" {
		t.Fatalf("parallel ready = %#v", second)
	}
	if err := s.Complete("backend"); err != nil {
		t.Fatal(err)
	}
	if ready := s.Ready(); len(ready) != 0 {
		t.Fatalf("qa should still wait for frontend: %#v", ready)
	}
	if err := s.Complete("frontend"); err != nil {
		t.Fatal(err)
	}
	third := s.AcquireReady(10)
	if len(third) != 1 || third[0].ID != "qa" {
		t.Fatalf("qa = %#v", third)
	}
}

func TestSchedulerPreventsOverlappingMutatingScopes(t *testing.T) {
	s, err := New([]Task{
		{ID: "backend-a", AgentID: "backend-engineer", Mutating: true, Scopes: []string{"backend/api"}},
		{ID: "backend-b", AgentID: "database-engineer", Mutating: true, Scopes: []string{"backend"}},
		{ID: "research", AgentID: "researcher"},
	})
	if err != nil {
		t.Fatal(err)
	}

	acquired := s.AcquireReady(10)
	if len(acquired) != 2 {
		t.Fatalf("acquired = %#v", acquired)
	}
	if acquired[0].ID != "backend-a" || acquired[1].ID != "research" {
		t.Fatalf("unexpected deterministic acquisition: %#v", acquired)
	}

	snapshot := s.Snapshot()
	status := map[string]Status{}
	for _, task := range snapshot {
		status[task.ID] = task.Status
	}
	if status["backend-b"] != StatusWaiting {
		t.Fatalf("backend-b status = %s", status["backend-b"])
	}
	if err := s.Complete("backend-a"); err != nil {
		t.Fatal(err)
	}
	ready := s.Ready()
	if len(ready) != 1 || ready[0].ID != "backend-b" {
		t.Fatalf("backend-b did not become ready: %#v", ready)
	}
}

func TestSchedulerFailureBlocksDependentsButNotIndependentTasks(t *testing.T) {
	s, err := New([]Task{
		{ID: "backend", AgentID: "backend-engineer"},
		{ID: "qa", AgentID: "qa-engineer", DependsOn: []string{"backend"}},
		{ID: "research", AgentID: "researcher"},
	})
	if err != nil {
		t.Fatal(err)
	}
	acquired := s.AcquireReady(10)
	if len(acquired) != 2 {
		t.Fatalf("acquired = %#v", acquired)
	}
	if err := s.Fail("backend"); err != nil {
		t.Fatal(err)
	}
	progress := s.Progress()
	if progress.Blocked != 1 || progress.Failed != 1 || progress.Running != 1 {
		t.Fatalf("progress = %#v", progress)
	}
	if err := s.Complete("research"); err != nil {
		t.Fatal(err)
	}
	if !s.Done() {
		t.Fatalf("scheduler should be terminal: %#v", s.Progress())
	}
}

func TestSchedulerCancellationPropagatesToDependents(t *testing.T) {
	s, err := New([]Task{
		{ID: "architect", AgentID: "architect"},
		{ID: "backend", AgentID: "backend-engineer", DependsOn: []string{"architect"}},
		{ID: "qa", AgentID: "qa-engineer", DependsOn: []string{"backend"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Cancel("architect"); err != nil {
		t.Fatal(err)
	}
	progress := s.Progress()
	if progress.Cancelled != 1 || progress.Blocked != 2 {
		t.Fatalf("progress = %#v", progress)
	}
	if !s.Done() {
		t.Fatalf("scheduler should be done after dependency cancellation: %#v", progress)
	}
}

func TestSchedulerRejectsCyclesUnknownDependenciesAndUnsafeScopes(t *testing.T) {
	if _, err := New([]Task{
		{ID: "a", AgentID: "architect", DependsOn: []string{"b"}},
		{ID: "b", AgentID: "backend-engineer", DependsOn: []string{"a"}},
	}); err == nil {
		t.Fatal("expected cycle rejection")
	}

	if _, err := New([]Task{
		{ID: "a", AgentID: "architect", DependsOn: []string{"missing"}},
	}); err == nil {
		t.Fatal("expected unknown dependency rejection")
	}

	if _, err := New([]Task{
		{ID: "a", AgentID: "backend-engineer", Mutating: true, Scopes: []string{"../outside"}},
	}); err == nil {
		t.Fatal("expected unsafe scope rejection")
	}
}

func TestSchedulerAcquireLimitAndDeterministicOrder(t *testing.T) {
	s, err := New([]Task{
		{ID: "c", AgentID: "researcher"},
		{ID: "a", AgentID: "architect"},
		{ID: "b", AgentID: "qa-engineer"},
	})
	if err != nil {
		t.Fatal(err)
	}
	acquired := s.AcquireReady(2)
	if len(acquired) != 2 || acquired[0].ID != "a" || acquired[1].ID != "b" {
		t.Fatalf("acquired = %#v", acquired)
	}
	progress := s.Progress()
	if progress.Running != 2 || progress.Ready != 1 {
		t.Fatalf("progress = %#v", progress)
	}
}


func TestSchedulerPreservesAndClonesToolOperationMetadata(t *testing.T) {
	input := map[string][]string{
		"github": {"read-repository", "read-issues", "read-repository"},
	}
	s, err := New([]Task{{
		ID: "research", AgentID: "researcher",
		ToolOperations: input,
	}})
	if err != nil {
		t.Fatal(err)
	}
	input["github"][0] = "create-commit"
	snapshot := s.Snapshot()
	got := snapshot[0].ToolOperations["github"]
	if len(got) != 2 || got[0] != "read-issues" || got[1] != "read-repository" {
		t.Fatalf("tool operations=%#v", snapshot[0].ToolOperations)
	}
	got[0] = "mutated"
	again := s.Snapshot()[0].ToolOperations["github"]
	if again[0] != "read-issues" {
		t.Fatalf("scheduler metadata aliased through snapshot: %#v", again)
	}
}
