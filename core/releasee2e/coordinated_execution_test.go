package releasee2e_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/artifact"
	"github.com/FernandoVillegas27/ai-engineering-os/core/collaboration"
	"github.com/FernandoVillegas27/ai-engineering-os/core/contractverify"
	"github.com/FernandoVillegas27/ai-engineering-os/core/coordinator"
	"github.com/FernandoVillegas27/ai-engineering-os/core/integration"
	"github.com/FernandoVillegas27/ai-engineering-os/core/message"
	"github.com/FernandoVillegas27/ai-engineering-os/core/promotion"
	"github.com/FernandoVillegas27/ai-engineering-os/core/review"
	"github.com/FernandoVillegas27/ai-engineering-os/core/scheduler"
	"github.com/FernandoVillegas27/ai-engineering-os/core/staging"
	"github.com/FernandoVillegas27/ai-engineering-os/core/taskworkspace"
	"github.com/FernandoVillegas27/ai-engineering-os/core/verification"
)

type workerFunc func(context.Context, scheduler.Task) (coordinator.WorkResult, error)

func (f workerFunc) Execute(ctx context.Context, task scheduler.Task) (coordinator.WorkResult, error) {
	return f(ctx, task)
}

type passVerifier struct{}

func (passVerifier) ID() string { return "release-e2e-verifier" }

func (passVerifier) Verify(context.Context, string) (verification.Report, error) {
	return verification.Report{Status: verification.StatusPassed}, nil
}

type approvingReviewer struct {
	calls int
}

func (r *approvingReviewer) ID() string { return "release-e2e-reviewer" }

func (r *approvingReviewer) Review(context.Context, string, string, []string) (review.Report, error) {
	r.calls++
	return review.Report{Verdict: review.VerdictApproved, Summary: "independent release E2E review approved"}, nil
}

func TestReadOnlyMultiAgentCoordinationPersistsTypedHandoffs(t *testing.T) {
	const runID = "release-e2e-readonly"
	artifactStore, err := artifact.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	messageBus, err := message.NewBus(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	tasks := []scheduler.Task{
		{ID: "research", AgentID: "researcher"},
		{ID: "architect", AgentID: "architect", DependsOn: []string{"research"}},
		{ID: "backend", AgentID: "backend-engineer", DependsOn: []string{"architect"}},
		{ID: "frontend", AgentID: "frontend-engineer", DependsOn: []string{"architect"}},
		{ID: "qa", AgentID: "qa-engineer", DependsOn: []string{"backend", "frontend"}},
	}
	graph, err := scheduler.New(tasks)
	if err != nil {
		t.Fatal(err)
	}
	dependents := collaboration.BuildDependents(tasks)
	now := time.Date(2026, 11, 6, 20, 0, 0, 0, time.UTC)

	workers := map[string]coordinator.Worker{}
	for _, agentID := range []string{
		"researcher", "architect", "backend-engineer", "frontend-engineer", "qa-engineer",
	} {
		agentID := agentID
		workers[agentID] = collaboration.HandoffWorker{
			Inner: workerFunc(func(_ context.Context, task scheduler.Task) (coordinator.WorkResult, error) {
				return coordinator.WorkResult{Summary: agentID + " completed " + task.ID}, nil
			}),
			RunID:         runID,
			ArtifactStore: artifactStore,
			MessageBus:    messageBus,
			Dependents:    dependents,
			Now:           func() time.Time { return now },
		}
	}

	events := &coordinator.MemoryEventSink{}
	result, err := (coordinator.Coordinator{
		Config:  coordinator.Config{MaxConcurrency: 3},
		Workers: workers,
		Events:  events,
	}).Run(context.Background(), graph)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Completed || result.Progress.Completed != len(tasks) ||
		result.Progress.Failed != 0 || result.Progress.Blocked != 0 {
		t.Fatalf("result=%#v", result)
	}

	artifacts, err := artifactStore.ListRun(runID)
	if err != nil {
		t.Fatal(err)
	}
	if len(artifacts) != len(tasks) {
		t.Fatalf("artifacts=%d want=%d", len(artifacts), len(tasks))
	}
	for _, value := range artifacts {
		if value.Type != artifact.TypeHandoff {
			t.Fatalf("unexpected artifact=%#v", value)
		}
		if schema := artifact.PayloadSchemaID(value.Type, value.Payload); schema != "artifact/handoff/v1" {
			t.Fatalf("artifact schema=%q", schema)
		}
	}

	messages, err := messageBus.List(runID, true, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	// research->architect, architect->backend/frontend, backend->qa, frontend->qa
	if len(messages) != 5 {
		t.Fatalf("messages=%d want=5", len(messages))
	}
	for _, value := range messages {
		if value.Kind != message.KindHandoff {
			t.Fatalf("unexpected message=%#v", value)
		}
		if schema := message.PayloadSchemaID(value.Kind, value.Payload); schema != "message/handoff/v1" {
			t.Fatalf("message schema=%q", schema)
		}
	}

	recorded := events.Snapshot()
	if len(recorded) == 0 ||
		recorded[0].Type != coordinator.EventRunStarted ||
		recorded[len(recorded)-1].Type != coordinator.EventRunCompleted {
		t.Fatalf("events=%#v", recorded)
	}
}

func TestMutationPipelineKeepsOriginalImmutableUntilExplicitPromotion(t *testing.T) {
	base := t.TempDir()
	original := filepath.Join(base, "project")
	taskParent := filepath.Join(base, "tasks")
	integrationParent := filepath.Join(base, "integration")
	for _, dir := range []string{original, taskParent, integrationParent} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(t, original, "backend/service.go", "package backend\nconst Version = 1\n")
	writeFile(t, original, "frontend/app.go", "package frontend\nconst Version = 1\n")

	manager, err := taskworkspace.New(taskworkspace.Config{
		OriginalRoot: original,
		Parent:       taskParent,
	})
	if err != nil {
		t.Fatal(err)
	}

	backend, err := manager.Prepare(
		context.Background(),
		"backend",
		"backend-engineer",
		[]staging.WorkspaceChange{{Path: "backend/service.go", Action: "modify"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Cleanup(backend)

	frontend, err := manager.Prepare(
		context.Background(),
		"frontend",
		"frontend-engineer",
		[]staging.WorkspaceChange{{Path: "frontend/app.go", Action: "modify"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Cleanup(frontend)

	writeFile(t, backend.Root, "backend/service.go", "package backend\nconst Version = 2\n")
	writeFile(t, frontend.Root, "frontend/app.go", "package frontend\nconst Version = 2\n")

	backendInspection, err := manager.Inspect(context.Background(), backend)
	if err != nil {
		t.Fatal(err)
	}
	frontendInspection, err := manager.Inspect(context.Background(), frontend)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := integration.Build(
		[]taskworkspace.Workspace{backend, frontend},
		map[string]taskworkspace.Inspection{
			"backend":  backendInspection,
			"frontend": frontendInspection,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	integrated, err := integration.Materialize(
		context.Background(),
		integration.MaterializeConfig{
			OriginalRoot: original,
			Parent:       integrationParent,
		},
		plan,
		[]taskworkspace.Workspace{backend, frontend},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer integration.Cleanup(integrated, integrationParent)

	assertFile(t, original, "backend/service.go", "package backend\nconst Version = 1\n")
	assertFile(t, original, "frontend/app.go", "package frontend\nconst Version = 1\n")

	reviewer := &approvingReviewer{}
	gate, err := integration.RunQualityGate(context.Background(), integration.QualityGateConfig{
		OriginalRoot: original,
		Plan:         plan,
		Workspace:    integrated,
		Task:         "release E2E integration",
		Verifier:     passVerifier{},
		Reviewer:     reviewer,
		MaxAttempts:  1,
		TempParent:   integrationParent,
		ContractVerifier: func(string, string) (contractverify.Result, error) {
			return contractverify.Result{Passed: true}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if reviewer.calls != 1 || !gate.Promotable || !gate.Quality.Passed || !gate.Contracts.Passed {
		t.Fatalf("reviewer.calls=%d gate=%#v", reviewer.calls, gate)
	}

	candidate, err := integration.BuildPromotionCandidate(
		original,
		integrated,
		gate,
		integrationParent,
	)
	if err != nil {
		t.Fatal(err)
	}
	dryRun, err := promotion.DryRun(context.Background(), candidate.Request)
	if err != nil {
		t.Fatal(err)
	}
	if !dryRun.Ready || len(dryRun.Changes) != 2 {
		t.Fatalf("dryRun=%#v", dryRun)
	}

	// Dry-run and an unapproved Apply must not mutate the original tree.
	assertFile(t, original, "backend/service.go", "package backend\nconst Version = 1\n")
	_, err = promotion.Apply(context.Background(), promotion.ApplyRequest{
		Request:  candidate.Request,
		Approved: false,
	})
	var promotionErr *promotion.Error
	if !errors.As(err, &promotionErr) || promotionErr.Kind != promotion.KindApprovalRequired {
		t.Fatalf("unapproved apply err=%#v", err)
	}
	assertFile(t, original, "backend/service.go", "package backend\nconst Version = 1\n")

	applied, err := promotion.Apply(context.Background(), promotion.ApplyRequest{
		Request:  candidate.Request,
		Approved: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !applied.Applied || applied.RolledBack || len(applied.Changes) != 2 {
		t.Fatalf("applied=%#v", applied)
	}
	assertFile(t, original, "backend/service.go", "package backend\nconst Version = 2\n")
	assertFile(t, original, "frontend/app.go", "package frontend\nconst Version = 2\n")
}

func writeFile(t *testing.T, root, relative, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertFile(t *testing.T, root, relative, want string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); got != want {
		t.Fatalf("%s=%q want=%q", relative, got, want)
	}
	if strings.TrimSpace(got) == "" {
		t.Fatalf("%s unexpectedly empty", relative)
	}
}
