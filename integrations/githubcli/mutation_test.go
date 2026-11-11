package githubcli

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/toolapproval"
)

func approvedPullRequest(t *testing.T, request CreatePullRequestRequest) toolapproval.Evidence {
	t.Helper()
	digest, err := request.RequestDigest()
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := toolapproval.New(toolapproval.Request{
		RunID: request.RunID,
		TaskID: request.TaskID,
		ToolID: "github",
		Operation: "create-pull-request",
		RequestDigest: digest,
		Status: toolapproval.StatusGranted,
		Actor: "human",
	}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	return evidence
}

func pullRequestFixture(t *testing.T) CreatePullRequestRequest {
	t.Helper()
	return CreatePullRequestRequest{
		RunID: "run-1",
		TaskID: "task-1",
		Root: t.TempDir(),
		Repository: "acme/project",
		Head: "feature/safe-pr",
		Base: "main",
		Title: "Add safe feature",
		Body: "Reviewed body",
	}
}

func TestCreatePullRequestRequiresExactApprovalBeforeAnyGitHubInvocation(t *testing.T) {
	request := pullRequestFixture(t)
	approval := approvedPullRequest(t, request)
	request.Title = "Changed after approval"

	runner := happyRunner()
	client := New()
	client.Runner = runner

	if _, err := client.CreatePullRequest(context.Background(), request, approval); err == nil ||
		!strings.Contains(err.Error(), "matching non-expired human approval") {
		t.Fatalf("err=%v", err)
	}
	if len(runner.invocations) != 0 {
		t.Fatalf("GitHub invoked before exact approval validation: %#v", runner.invocations)
	}
}

func TestCreatePullRequestUsesFixedCommandAfterRepoIdentityVerification(t *testing.T) {
	request := pullRequestFixture(t)
	approval := approvedPullRequest(t, request)
	runner := happyRunner()
	runner.responses["repo view --json nameWithOwner"] = `{"nameWithOwner":"acme/project"}`
	expected := strings.Join([]string{
		"pr", "create",
		"--repo", "acme/project",
		"--head", "feature/safe-pr",
		"--base", "main",
		"--title", "Add safe feature",
		"--body", "Reviewed body",
	}, " ")
	runner.responses[expected] = "https://github.com/acme/project/pull/42"

	client := New()
	client.Runner = runner
	result, err := client.CreatePullRequest(context.Background(), request, approval)
	if err != nil {
		t.Fatal(err)
	}
	if result.URL != "https://github.com/acme/project/pull/42" {
		t.Fatalf("result=%#v", result)
	}
	if len(runner.invocations) != 3 {
		t.Fatalf("invocations=%#v", runner.invocations)
	}
	got := strings.Join(runner.invocations[2].Args, " ")
	if got != expected {
		t.Fatalf("mutation argv=%q want=%q", got, expected)
	}
}

func TestCreatePullRequestRepositoryMismatchPreventsMutation(t *testing.T) {
	request := pullRequestFixture(t)
	approval := approvedPullRequest(t, request)
	runner := happyRunner()
	runner.responses["repo view --json nameWithOwner"] = `{"nameWithOwner":"other/project"}`

	client := New()
	client.Runner = runner
	if _, err := client.CreatePullRequest(context.Background(), request, approval); err == nil ||
		!strings.Contains(err.Error(), "identity mismatch") {
		t.Fatalf("err=%v", err)
	}
	if len(runner.invocations) != 2 {
		t.Fatalf("unexpected mutation invocation: %#v", runner.invocations)
	}
}

func TestCreatePullRequestRejectsUnsafeInputsBeforeGitHubInvocation(t *testing.T) {
	cases := []CreatePullRequestRequest{
		{RunID: "run-1", TaskID: "task-1", Root: t.TempDir(), Repository: "../repo", Head: "feature/a", Base: "main", Title: "x"},
		{RunID: "run-1", TaskID: "task-1", Root: t.TempDir(), Repository: "acme/project", Head: "-danger", Base: "main", Title: "x"},
		{RunID: "run-1", TaskID: "task-1", Root: t.TempDir(), Repository: "acme/project", Head: "feature/a", Base: "main", Title: "line1\nline2"},
	}
	for i, request := range cases {
		runner := happyRunner()
		client := New()
		client.Runner = runner
		approval := toolapproval.Evidence{}
		if _, err := client.CreatePullRequest(context.Background(), request, approval); err == nil {
			t.Fatalf("case %d expected rejection", i)
		}
		if len(runner.invocations) != 0 {
			t.Fatalf("case %d invoked GitHub: %#v", i, runner.invocations)
		}
	}
}

func TestPullRequestDigestChangesWithEveryMutationRelevantField(t *testing.T) {
	base := pullRequestFixture(t)
	original, err := base.RequestDigest()
	if err != nil {
		t.Fatal(err)
	}
	variants := []CreatePullRequestRequest{
		base, base, base, base, base,
	}
	variants[0].Repository = "acme/other"
	variants[1].Head = "feature/other"
	variants[2].Base = "develop"
	variants[3].Title = "Other title"
	variants[4].Body = "Other body"
	for i, variant := range variants {
		got, err := variant.RequestDigest()
		if err != nil {
			t.Fatal(err)
		}
		if got == original {
			t.Fatalf("variant %d did not change digest", i)
		}
	}
}


func TestPullRequestResultURLMatchesExactApprovedGitHubRepository(t *testing.T) {
	valid := []string{
		"https://github.com/acme/project/pull/42",
		"https://github.com/acme/project/pull/1",
	}
	for _, value := range valid {
		if err := validatePullRequestResultURL(value, "acme/project"); err != nil {
			t.Errorf("valid URL %q rejected: %v", value, err)
		}
	}
	invalid := []string{
		"https://evil.example/acme/project/pull/42",
		"https://github.com.evil.example/acme/project/pull/42",
		"https://attacker@github.com/acme/project/pull/42",
		"http://github.com/acme/project/pull/42",
		"https://github.com/acme/other/pull/42",
		"https://github.com/other/project/pull/42",
		"https://github.com/acme/project/pull/42/files",
		"https://github.com/acme/project/pull/0",
		"https://github.com/acme/project/pull/-1",
		"https://github.com/acme/project/pull/01",
		"https://github.com/acme/project/pull/not-a-number",
		"https://github.com/acme/project/pull/42?next=evil",
		"https://github.com/acme/project/pull/42#fragment",
		"https://github.com/acme/project/pull/42/",
		"https://github.com/acme/project/pull/42%2Ffiles",
		"https://github.com/acme/project/pull/%34%32",
		"https://github.com/acme/project/pull/42\\foo",
		"https://github.com/acme/project/pull/18446744073709551616",
		"https://github.com/acme/project/pull/42\nhttps://evil.example",
		"",
	}
	for _, value := range invalid {
		if err := validatePullRequestResultURL(value, "acme/project"); err == nil {
			t.Errorf("unsafe or unrelated URL accepted: %q", value)
		}
	}
}

func TestCreatePullRequestRejectsUntrustedReturnedURL(t *testing.T) {
	request := pullRequestFixture(t)
	approval := approvedPullRequest(t, request)
	runner := happyRunner()
	runner.responses["repo view --json nameWithOwner"] = `{"nameWithOwner":"acme/project"}`
	command := strings.Join([]string{
		"pr", "create", "--repo", "acme/project", "--head", "feature/safe-pr",
		"--base", "main", "--title", "Add safe feature", "--body", "Reviewed body",
	}, " ")
	runner.responses[command] = "https://github.com/attacker/repository/pull/7"
	client := New()
	client.Runner = runner
	if _, err := client.CreatePullRequest(context.Background(), request, approval); err == nil ||
		!strings.Contains(err.Error(), "invalid pull-request URL") {
		t.Fatalf("expected a rejected untrusted CLI response, err=%v", err)
	}
}
