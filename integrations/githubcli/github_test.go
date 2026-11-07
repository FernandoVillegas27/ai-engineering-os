package githubcli

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeRunner struct {
	path string
	lookErr error
	responses map[string]string
	errors map[string]error
	invocations []Invocation
}

func (f *fakeRunner) LookPath(string) (string, error) {
	if f.lookErr != nil {
		return "", f.lookErr
	}
	return f.path, nil
}

func (f *fakeRunner) Run(_ context.Context, invocation Invocation) (string, error) {
	f.invocations = append(f.invocations, invocation)
	key := strings.Join(invocation.Args, " ")
	return f.responses[key], f.errors[key]
}

func happyRunner() *fakeRunner {
	return &fakeRunner{
		path: "/usr/bin/gh",
		responses: map[string]string{
			"auth status": "github.com logged in",
			"repo view --json nameWithOwner,url,defaultBranchRef,isPrivate": `{"nameWithOwner":"acme/project","url":"https://github.com/acme/project","isPrivate":true,"defaultBranchRef":{"name":"main"}}`,
			"issue list --state open --limit 20 --json number,title,state,updatedAt,url,labels": `[{"number":12,"title":"Fix cache","state":"OPEN"}]`,
			"pr list --state open --limit 20 --json number,title,state,isDraft,updatedAt,url,headRefName,baseRefName": `[{"number":8,"title":"Feature","state":"OPEN","isDraft":false}]`,
		},
		errors: map[string]error{},
	}
}

func TestSnapshotBindsOnlyReadOperations(t *testing.T) {
	runner := happyRunner()
	client := New()
	client.Runner = runner

	result, err := client.Snapshot(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(result.BoundOperations, ","); got != "read-issues,read-pull-requests,read-repository" {
		t.Fatalf("operations=%q", got)
	}
	if !strings.Contains(result.Context, "untrusted external data") ||
		!strings.Contains(result.Context, "Fix cache") {
		t.Fatalf("context=%q", result.Context)
	}
	for _, invocation := range runner.invocations {
		args := strings.Join(invocation.Args, " ")
		if strings.Contains(args, "create") || strings.Contains(args, "merge") ||
			strings.Contains(args, "edit") || strings.Contains(args, "delete") {
			t.Fatalf("write-capable GitHub command executed: %q", args)
		}
	}
}

func TestSnapshotSupportsPartialReadBinding(t *testing.T) {
	runner := happyRunner()
	runner.errors["issue list --state open --limit 20 --json number,title,state,updatedAt,url,labels"] = errors.New("403 forbidden")
	client := New()
	client.Runner = runner

	result, err := client.Snapshot(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(result.BoundOperations, ","); got != "read-pull-requests,read-repository" {
		t.Fatalf("operations=%q", got)
	}
	if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "read-issues") {
		t.Fatalf("warnings=%#v", result.Warnings)
	}
}

func TestSnapshotFailsClosedWhenAuthenticationFails(t *testing.T) {
	runner := happyRunner()
	runner.errors["auth status"] = errors.New("not authenticated")
	client := New()
	client.Runner = runner

	if _, err := client.Snapshot(context.Background(), t.TempDir()); err == nil ||
		!strings.Contains(err.Error(), "authentication") {
		t.Fatalf("err=%v", err)
	}
	if len(runner.invocations) != 1 {
		t.Fatalf("invocations=%#v", runner.invocations)
	}
}

func TestSnapshotControlledEnvironmentDoesNotLeakUnrelatedSecrets(t *testing.T) {
	t.Setenv("AWS_SECRET_ACCESS_KEY", "secret")
	t.Setenv("GH_TOKEN", "gh-test-token")
	runner := happyRunner()
	client := New()
	client.Runner = runner

	if _, err := client.Snapshot(context.Background(), t.TempDir()); err != nil {
		t.Fatal(err)
	}
	env := strings.Join(runner.invocations[0].Env, "\n")
	if strings.Contains(env, "AWS_SECRET_ACCESS_KEY") {
		t.Fatalf("unrelated secret leaked: %q", env)
	}
	if !strings.Contains(env, "GH_TOKEN=gh-test-token") {
		t.Fatalf("GitHub credential missing from trusted adapter env: %q", env)
	}
	if !strings.Contains(env, "GH_PROMPT_DISABLED=1") {
		t.Fatalf("non-interactive boundary missing: %q", env)
	}
}

func TestSnapshotRejectsInvalidJSONAndNoEvidence(t *testing.T) {
	runner := happyRunner()
	runner.responses["repo view --json nameWithOwner,url,defaultBranchRef,isPrivate"] = "not-json"
	runner.responses["issue list --state open --limit 20 --json number,title,state,updatedAt,url,labels"] = "bad"
	runner.responses["pr list --state open --limit 20 --json number,title,state,isDraft,updatedAt,url,headRefName,baseRefName"] = "bad"
	client := New()
	client.Runner = runner

	if _, err := client.Snapshot(context.Background(), t.TempDir()); err == nil ||
		!strings.Contains(err.Error(), "no read operation") {
		t.Fatalf("err=%v", err)
	}
}
