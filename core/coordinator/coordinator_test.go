package coordinator

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/scheduler"
)

type workerFunc func(context.Context, scheduler.Task) (WorkResult, error)

func (f workerFunc) Execute(ctx context.Context, task scheduler.Task) (WorkResult, error) {
	return f(ctx, task)
}

func TestCoordinatorRunsDependenciesInOrder(t *testing.T) {
	graph, err := scheduler.New([]scheduler.Task{
		{ID: "architect", AgentID: "architect"},
		{ID: "backend", AgentID: "backend-engineer", DependsOn: []string{"architect"}},
		{ID: "qa", AgentID: "qa-engineer", DependsOn: []string{"backend"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var order []string
	worker := func(agent string) Worker {
		return workerFunc(func(_ context.Context, task scheduler.Task) (WorkResult, error) {
			mu.Lock()
			order = append(order, task.ID)
			mu.Unlock()
			return WorkResult{Summary: agent + " done"}, nil
		})
	}
	events := &MemoryEventSink{}
	result, err := (Coordinator{
		Config: Config{MaxConcurrency: 3},
		Workers: map[string]Worker{
			"architect":        worker("architect"),
			"backend-engineer": worker("backend"),
			"qa-engineer":      worker("qa"),
		},
		Events: events,
	}).Run(context.Background(), graph)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Completed || result.Progress.Completed != 3 {
		t.Fatalf("result = %#v", result)
	}
	if len(order) != 3 || order[0] != "architect" || order[1] != "backend" || order[2] != "qa" {
		t.Fatalf("order = %#v", order)
	}
	recorded := events.Snapshot()
	if len(recorded) != 8 {
		t.Fatalf("events = %#v", recorded)
	}
	if recorded[0].Type != EventRunStarted || recorded[len(recorded)-1].Type != EventRunCompleted {
		t.Fatalf("events = %#v", recorded)
	}
}

func TestCoordinatorRunsIndependentWorkersConcurrently(t *testing.T) {
	graph, err := scheduler.New([]scheduler.Task{
		{ID: "backend", AgentID: "backend-engineer", Mutating: true, Scopes: []string{"backend"}},
		{ID: "frontend", AgentID: "frontend-engineer", Mutating: true, Scopes: []string{"frontend"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	release := make(chan struct{})
	started := make(chan struct{}, 2)
	var active int32
	var maxActive int32
	worker := workerFunc(func(_ context.Context, task scheduler.Task) (WorkResult, error) {
		current := atomic.AddInt32(&active, 1)
		for {
			previous := atomic.LoadInt32(&maxActive)
			if current <= previous || atomic.CompareAndSwapInt32(&maxActive, previous, current) {
				break
			}
		}
		started <- struct{}{}
		<-release
		atomic.AddInt32(&active, -1)
		return WorkResult{Summary: task.ID}, nil
	})

	done := make(chan error, 1)
	go func() {
		_, runErr := (Coordinator{
			Config: Config{MaxConcurrency: 2},
			Workers: map[string]Worker{
				"backend-engineer":  worker,
				"frontend-engineer": worker,
			},
		}).Run(context.Background(), graph)
		done <- runErr
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("first worker did not start")
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("second worker did not start concurrently")
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt32(&maxActive) != 2 {
		t.Fatalf("max active = %d, want 2", maxActive)
	}
}

func TestCoordinatorSchedulerPreventsOverlappingMutations(t *testing.T) {
	graph, err := scheduler.New([]scheduler.Task{
		{ID: "a", AgentID: "backend-engineer", Mutating: true, Scopes: []string{"backend"}},
		{ID: "b", AgentID: "database-engineer", Mutating: true, Scopes: []string{"backend/db"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	var active int32
	var maxActive int32
	worker := workerFunc(func(_ context.Context, task scheduler.Task) (WorkResult, error) {
		current := atomic.AddInt32(&active, 1)
		if current > atomic.LoadInt32(&maxActive) {
			atomic.StoreInt32(&maxActive, current)
		}
		time.Sleep(10 * time.Millisecond)
		atomic.AddInt32(&active, -1)
		return WorkResult{Summary: task.ID}, nil
	})
	result, err := (Coordinator{
		Config: Config{MaxConcurrency: 2},
		Workers: map[string]Worker{
			"backend-engineer":  worker,
			"database-engineer": worker,
		},
	}).Run(context.Background(), graph)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Completed || atomic.LoadInt32(&maxActive) != 1 {
		t.Fatalf("result=%#v maxActive=%d", result, maxActive)
	}
}

func TestCoordinatorFailureBlocksDependentAndContinuesIndependentBranch(t *testing.T) {
	graph, err := scheduler.New([]scheduler.Task{
		{ID: "backend", AgentID: "backend-engineer"},
		{ID: "qa", AgentID: "qa-engineer", DependsOn: []string{"backend"}},
		{ID: "research", AgentID: "researcher"},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := (Coordinator{
		Config: Config{MaxConcurrency: 2},
		Workers: map[string]Worker{
			"backend-engineer": workerFunc(func(context.Context, scheduler.Task) (WorkResult, error) {
				return WorkResult{}, errors.New("backend failed")
			}),
			"qa-engineer": workerFunc(func(context.Context, scheduler.Task) (WorkResult, error) {
				t.Fatal("qa must not run")
				return WorkResult{}, nil
			}),
			"researcher": workerFunc(func(_ context.Context, task scheduler.Task) (WorkResult, error) {
				return WorkResult{Summary: "research complete"}, nil
			}),
		},
	}).Run(context.Background(), graph)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Completed {
		t.Fatalf("result = %#v", result)
	}
	if result.Progress.Failed != 1 || result.Progress.Blocked != 1 || result.Progress.Completed != 1 {
		t.Fatalf("progress = %#v", result.Progress)
	}
}

func TestCoordinatorMissingWorkerFailsTaskWithoutPreventingIndependentWork(t *testing.T) {
	graph, err := scheduler.New([]scheduler.Task{
		{ID: "architect", AgentID: "architect"},
		{ID: "research", AgentID: "researcher"},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := (Coordinator{
		Config: Config{MaxConcurrency: 1},
		Workers: map[string]Worker{
			"researcher": workerFunc(func(context.Context, scheduler.Task) (WorkResult, error) {
				return WorkResult{Summary: "done"}, nil
			}),
		},
	}).Run(context.Background(), graph)
	if err != nil {
		t.Fatal(err)
	}
	if result.Progress.Failed != 1 || result.Progress.Completed != 1 {
		t.Fatalf("result = %#v", result)
	}
}

func TestCoordinatorCancellationCancelsRemainingGraph(t *testing.T) {
	graph, err := scheduler.New([]scheduler.Task{
		{ID: "backend", AgentID: "backend-engineer"},
		{ID: "qa", AgentID: "qa-engineer", DependsOn: []string{"backend"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	worker := workerFunc(func(ctx context.Context, task scheduler.Task) (WorkResult, error) {
		close(started)
		<-ctx.Done()
		return WorkResult{}, ctx.Err()
	})
	done := make(chan Result, 1)
	errs := make(chan error, 1)
	go func() {
		result, runErr := (Coordinator{
			Workers: map[string]Worker{"backend-engineer": worker, "qa-engineer": worker},
		}).Run(ctx, graph)
		done <- result
		errs <- runErr
	}()
	<-started
	cancel()
	result := <-done
	runErr := <-errs
	if runErr == nil || !errors.Is(runErr, context.Canceled) {
		t.Fatalf("err = %#v", runErr)
	}
	if !result.Cancelled {
		t.Fatalf("result = %#v", result)
	}
	if result.Progress.Cancelled != 2 {
		t.Fatalf("progress = %#v", result.Progress)
	}
}
