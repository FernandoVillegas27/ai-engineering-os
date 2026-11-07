package main

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/controlplane"
	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/runstate"
	"github.com/FernandoVillegas27/ai-engineering-os/core/toolapproval"
	githubintegration "github.com/FernandoVillegas27/ai-engineering-os/integrations/githubcli"
)

type fakeGitHubPRCreator struct {
	calls int
	request githubintegration.CreatePullRequestRequest
	approval toolapproval.Evidence
	result githubintegration.CreatePullRequestResult
	err error
}

func (f *fakeGitHubPRCreator) CreatePullRequest(
	_ context.Context,
	request githubintegration.CreatePullRequestRequest,
	approval toolapproval.Evidence,
) (githubintegration.CreatePullRequestResult, error) {
	f.calls++
	f.request = request
	f.approval = approval
	return f.result, f.err
}

func setupGitHubPRRun(t *testing.T) (runstate.Run, string, string, string, string) {
	t.Helper()
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("AI_ENGINEERING_OS_HOME", repoRoot)

	root := t.TempDir()
	stateDir := filepath.Join(root, "runs")
	eventsDir := filepath.Join(root, "events")
	approvalDir := filepath.Join(root, "approvals")
	projectRoot := t.TempDir()

	store, err := runstate.NewStore(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	run, err := runstate.New(runstate.NewRunRequest{
		Task: "open reviewed pull request",
		ProjectID: "project-1",
		ProjectRoot: projectRoot,
		WorkUnits: []execution.WorkUnit{{
			ID: "reviewer-1",
			AgentID: "reviewer",
			Task: "open reviewed pull request",
			Tools: []string{"github"},
			ToolOperations: map[string][]string{
				"github": {"read-repository"},
			},
		}},
	}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(run); err != nil {
		t.Fatal(err)
	}

	eventStore, err := controlplane.NewEventStore(eventsDir)
	if err != nil {
		t.Fatal(err)
	}
	started, err := controlplane.NewEvent(controlplane.NewEventRequest{
		RunID: run.ID,
		Type: controlplane.EventRunStarted,
		Data: map[string]any{"source": "test"},
	}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := eventStore.Append(context.Background(), started); err != nil {
		t.Fatal(err)
	}
	return run, stateDir, eventsDir, approvalDir, projectRoot
}

func githubPRArgs(runID, stateDir, eventsDir string) []string {
	return []string{
		runID,
		"--task", "reviewer-1",
		"--repository", "acme/project",
		"--head", "feature/safe-pr",
		"--base", "main",
		"--title", "Add reviewed feature",
		"--body", "Reviewed body",
		"--state-dir", stateDir,
		"--events-dir", eventsDir,
	}
}

func TestGitHubPRPrepareCreatesExactPendingApproval(t *testing.T) {
	run, stateDir, eventsDir, _, projectRoot := setupGitHubPRRun(t)
	args := githubPRArgs(run.ID, stateDir, eventsDir)
	if err := runRunGitHubPRPrepare(args); err != nil {
		t.Fatal(err)
	}

	request := githubintegration.CreatePullRequestRequest{
		RunID: run.ID,
		TaskID: "reviewer-1",
		Root: projectRoot,
		Repository: "acme/project",
		Head: "feature/safe-pr",
		Base: "main",
		Title: "Add reviewed feature",
		Body: "Reviewed body",
	}
	digest, err := request.RequestDigest()
	if err != nil {
		t.Fatal(err)
	}

	eventStore, err := controlplane.NewEventStore(eventsDir)
	if err != nil {
		t.Fatal(err)
	}
	events, err := eventStore.List(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[1].Type != controlplane.EventApprovalRequested {
		t.Fatalf("events=%#v", events)
	}
	state, err := controlplane.Project(events)
	if err != nil {
		t.Fatal(err)
	}
	if state.PendingApprovals != 1 || len(state.ToolApprovals) != 1 {
		t.Fatalf("state=%#v", state)
	}
	approval := state.ToolApprovals[0]
	if approval.ToolID != "github" ||
		approval.Operation != "create-pull-request" ||
		approval.RequestDigest != digest ||
		approval.Status != "pending" {
		t.Fatalf("approval=%#v", approval)
	}
}

func TestGitHubPRCreateUsesExactApprovalAndAuditsMutation(t *testing.T) {
	run, stateDir, eventsDir, approvalDir, projectRoot := setupGitHubPRRun(t)
	request := githubintegration.CreatePullRequestRequest{
		RunID: run.ID,
		TaskID: "reviewer-1",
		Root: projectRoot,
		Repository: "acme/project",
		Head: "feature/safe-pr",
		Base: "main",
		Title: "Add reviewed feature",
		Body: "Reviewed body",
	}
	digest, err := request.RequestDigest()
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := toolapproval.New(toolapproval.Request{
		RunID: run.ID,
		TaskID: "reviewer-1",
		ToolID: "github",
		Operation: "create-pull-request",
		RequestDigest: digest,
		Status: toolapproval.StatusGranted,
		Actor: "human",
	}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	approvalStore, err := toolapproval.NewStore(approvalDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := approvalStore.Save(evidence); err != nil {
		t.Fatal(err)
	}

	fake := &fakeGitHubPRCreator{
		result: githubintegration.CreatePullRequestResult{
			URL: "https://github.com/acme/project/pull/42",
			RequestDigest: digest,
		},
	}
	originalFactory := newGitHubPRCreator
	newGitHubPRCreator = func() githubPRCreator { return fake }
	t.Cleanup(func() { newGitHubPRCreator = originalFactory })

	args := append(githubPRArgs(run.ID, stateDir, eventsDir), "--approval-dir", approvalDir)
	if err := runRunGitHubPRCreate(args); err != nil {
		t.Fatal(err)
	}
	if fake.calls != 1 {
		t.Fatalf("calls=%d", fake.calls)
	}
	if fake.request.Repository != "acme/project" || fake.request.Root != projectRoot {
		t.Fatalf("request=%#v", fake.request)
	}
	if fake.approval.ID != evidence.ID {
		t.Fatalf("approval=%#v", fake.approval)
	}

	eventStore, err := controlplane.NewEventStore(eventsDir)
	if err != nil {
		t.Fatal(err)
	}
	events, err := eventStore.List(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 ||
		events[1].Type != controlplane.EventToolStarted ||
		events[2].Type != controlplane.EventToolCompleted {
		t.Fatalf("events=%#v", events)
	}
}

func TestGitHubPRCreateDoesNotInvokeAdapterForChangedRequest(t *testing.T) {
	run, stateDir, eventsDir, approvalDir, projectRoot := setupGitHubPRRun(t)
	approved := githubintegration.CreatePullRequestRequest{
		RunID: run.ID,
		TaskID: "reviewer-1",
		Root: projectRoot,
		Repository: "acme/project",
		Head: "feature/safe-pr",
		Base: "main",
		Title: "Approved title",
		Body: "Reviewed body",
	}
	digest, err := approved.RequestDigest()
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := toolapproval.New(toolapproval.Request{
		RunID: run.ID,
		TaskID: "reviewer-1",
		ToolID: "github",
		Operation: "create-pull-request",
		RequestDigest: digest,
		Status: toolapproval.StatusGranted,
		Actor: "human",
	}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	store, err := toolapproval.NewStore(approvalDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(evidence); err != nil {
		t.Fatal(err)
	}

	fake := &fakeGitHubPRCreator{}
	originalFactory := newGitHubPRCreator
	newGitHubPRCreator = func() githubPRCreator { return fake }
	t.Cleanup(func() { newGitHubPRCreator = originalFactory })

	args := githubPRArgs(run.ID, stateDir, eventsDir)
	for i := range args {
		if args[i] == "Add reviewed feature" {
			args[i] = "Changed after approval"
		}
	}
	args = append(args, "--approval-dir", approvalDir)
	if err := runRunGitHubPRCreate(args); err == nil {
		t.Fatal("expected exact approval mismatch")
	}
	if fake.calls != 0 {
		t.Fatalf("adapter invoked without matching request approval: %d", fake.calls)
	}
}
