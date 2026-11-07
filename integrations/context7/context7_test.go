package context7

import (
	"context"
	"errors"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/toolhealth"
)

type fakeRunner struct {
	path        string
	lookPathErr error
	version     string
	versionErr  error
	block       bool
	searchOutput string
	searchErr    error
	lastEnv      []string
	lastArgs     []string
}

func (f *fakeRunner) LookPath(file string) (string, error) {
	if f.lookPathErr != nil {
		return "", f.lookPathErr
	}
	return f.path, nil
}

func (f *fakeRunner) Run(ctx context.Context, _ string, args ...string) (string, error) {
	if f.block {
		<-ctx.Done()
		return "", ctx.Err()
	}
	if len(args) == 1 && args[0] == "--version" {
		return f.version, f.versionErr
	}
	return "", nil
}


func (f *fakeRunner) RunWithEnv(ctx context.Context, env []string, _ string, args ...string) (string, error) {
	f.lastEnv = append([]string(nil), env...)
	f.lastArgs = append([]string(nil), args...)
	if f.block {
		<-ctx.Done()
		return "", ctx.Err()
	}
	return f.searchOutput, f.searchErr
}

func TestStatusReadyWhenNPXExists(t *testing.T) {
	runner := &fakeRunner{
		path:    "C:/Program Files/nodejs/npx.cmd",
		version: "11.6.2",
	}
	client := Client{NPX: "npx", Runner: runner}

	status, err := client.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if !status.Available || !status.Ready {
		t.Fatalf("status = %#v", status)
	}
	if status.Version != "11.6.2" {
		t.Fatalf("version = %q", status.Version)
	}
}

func TestStatusMissingNPX(t *testing.T) {
	client := Client{
		NPX: "npx",
		Runner: &fakeRunner{
			lookPathErr: exec.ErrNotFound,
		},
	}

	status, err := client.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status.Available || status.Ready {
		t.Fatalf("status = %#v", status)
	}
}

func TestMCPConfigUsesPinnedPackage(t *testing.T) {
	got := New().MCP()
	wantArgs := []string{"-y", "@upstash/context7-mcp@4.4.2"}

	if got.Command != "npx" || !reflect.DeepEqual(got.Args, wantArgs) {
		t.Fatalf("MCP() = %#v", got)
	}
}

func TestRecommendedOperations(t *testing.T) {
	got := RecommendedOperations()
	want := []string{"resolve-library-id", "query-docs"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("RecommendedOperations() = %#v", got)
	}
}


func TestHealthClassifiesProbeTimeout(t *testing.T) {
	client := Client{
		NPX: "npx",
		Runner: &fakeRunner{path: "/usr/bin/npx", block: true},
		ProbeTimeout: 20 * time.Millisecond,
	}
	health := client.Health(context.Background())
	if health.Healthy || !health.Available {
		t.Fatalf("health=%#v", health)
	}
	if health.Failure.Kind != toolhealth.FailureTimeout || !health.Failure.Retryable {
		t.Fatalf("failure=%#v", health.Failure)
	}
}


func TestSearchUsesPinnedCLIControlledEnvironmentAndBoundedLibraries(t *testing.T) {
	runner := &fakeRunner{
		path: "/usr/bin/npx",
		searchOutput: `{"codeSnippets":[{"libraryId":"/angular/angular","codeTitle":"Signals"}],"infoSnippets":[]}`,
	}
	client := New()
	client.Runner = runner

	result, err := client.Search(context.Background(), SearchRequest{
		Query: "Current API usage and compatibility notes for detected project libraries.",
		Libraries: []string{"Angular", "Spring Boot", "Angular", "TypeScript", "React", "Next.js"},
		Language: "TypeScript",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Context, "untrusted data") ||
		!strings.Contains(result.Context, "/angular/angular") {
		t.Fatalf("context=%q", result.Context)
	}
	args := strings.Join(runner.lastArgs, " ")
	if !strings.Contains(args, "ctx7@0.10.0 search") ||
		!strings.Contains(args, "--json") ||
		strings.Count(args, "--library") != 4 {
		t.Fatalf("args=%q", args)
	}
	env := strings.Join(runner.lastEnv, "\n")
	if !strings.Contains(env, "CTX7_TELEMETRY_DISABLED=1") {
		t.Fatalf("controlled env missing telemetry disable: %q", env)
	}
	if strings.Contains(env, "CONTEXT7_API_KEY=") {
		t.Fatalf("API key must not be forwarded implicitly: %q", env)
	}
}

func TestSearchFailsClosedOnInvalidJSONOrProviderFailure(t *testing.T) {
	client := New()
	client.Runner = &fakeRunner{path: "/usr/bin/npx", searchOutput: "not-json"}
	if _, err := client.Search(context.Background(), SearchRequest{Query: "docs"}); err == nil ||
		!strings.Contains(err.Error(), "invalid JSON") {
		t.Fatalf("err=%v", err)
	}

	client.Runner = &fakeRunner{path: "/usr/bin/npx", searchErr: errors.New("429 rate limit")}
	if _, err := client.Search(context.Background(), SearchRequest{Query: "docs"}); err == nil ||
		!strings.Contains(err.Error(), "rate-limit") {
		t.Fatalf("err=%v", err)
	}
}

func TestSearchRequiresControlledEnvironmentRunner(t *testing.T) {
	client := New()
	client.Runner = structRunner{path: "/usr/bin/npx"}
	if _, err := client.Search(context.Background(), SearchRequest{Query: "docs"}); err == nil ||
		!strings.Contains(err.Error(), "controlled environment") {
		t.Fatalf("err=%v", err)
	}
}

type structRunner struct{ path string }
func (r structRunner) LookPath(string) (string, error) { return r.path, nil }
func (r structRunner) Run(context.Context, string, ...string) (string, error) { return "", nil }
