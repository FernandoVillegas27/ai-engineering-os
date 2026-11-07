package engram

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/toolhealth"

	"github.com/FernandoVillegas27/ai-engineering-os/core/memory"
)

type fakeRunner struct {
	path          string
	lookPathErr   error
	versionOutput string
	versionErr    error
	doctorOutput  string
	doctorErr     error
	calls         [][]string
	block         bool
}

func (f *fakeRunner) LookPath(file string) (string, error) {
	if f.lookPathErr != nil {
		return "", f.lookPathErr
	}
	return f.path, nil
}

func (f *fakeRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	if f.block {
		<-ctx.Done()
		return "", ctx.Err()
	}
	call := append([]string{name}, args...)
	f.calls = append(f.calls, call)

	if len(args) > 0 && args[0] == "version" {
		return f.versionOutput, f.versionErr
	}
	if len(args) > 0 && args[0] == "doctor" {
		return f.doctorOutput, f.doctorErr
	}
	if len(args) > 0 && args[0] == "save" {
		return "Memory saved: #42 \"test\" (decision)", nil
	}
	if len(args) > 0 && args[0] == "timeline" {
		return "Timeline for observation #42", nil
	}
	if len(args) > 0 && args[0] == "delete" {
		return "Observation #42 soft-deleted", nil
	}
	return "", nil
}

func TestStatusHealthy(t *testing.T) {
	runner := &fakeRunner{
		path:          "/usr/local/bin/engram",
		versionOutput: "engram v3.0.0",
		doctorOutput:  `{"status":"ok"}`,
	}

	client := Client{Binary: "engram", Runner: runner}
	status, err := client.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}

	if !status.Available || !status.Healthy {
		t.Fatalf("status = %#v", status)
	}
	if status.Version != "engram v3.0.0" {
		t.Fatalf("version = %q", status.Version)
	}
}

func TestStatusLookupFailureBecomesUnavailableHealth(t *testing.T) {
	runner := &fakeRunner{lookPathErr: errors.New("permission denied")}
	client := Client{Binary: "engram", Runner: runner}

	status, err := client.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.Available || status.Healthy {
		t.Fatalf("status=%#v", status)
	}
}

func TestMCPConfigUsesAgentProfile(t *testing.T) {
	client := New()
	got := client.MCP()

	wantArgs := []string{"mcp", "--tools=agent"}
	if got.Command != "engram" || !reflect.DeepEqual(got.Args, wantArgs) {
		t.Fatalf("MCP() = %#v", got)
	}
}

func TestSetupCommand(t *testing.T) {
	got, err := SetupCommand("cursor")
	if err != nil {
		t.Fatalf("SetupCommand() error = %v", err)
	}

	want := []string{"setup", "cursor"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SetupCommand() = %#v", got)
	}
}

func TestSetupCommandRejectsUnknownRuntime(t *testing.T) {
	if _, err := SetupCommand("unknown-runtime"); err == nil {
		t.Fatal("expected unsupported target error")
	}
}


func TestSaveUsesExplicitProjectAndCuratedType(t *testing.T) {
	runner := &fakeRunner{path: "/usr/local/bin/engram"}
	client := Client{Binary: "engram", Runner: runner}

	result, err := client.Save(context.Background(), memory.SaveRequest{
		Project:  "ai-engineering-os",
		Title:    "Chose typed agent messages",
		Content:  "Agents exchange typed messages and artifact references.",
		Type:     memory.TypeArchitecture,
		TopicKey: "architecture/agent-messages",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Saved || result.Project != "ai-engineering-os" || result.MemoryID != "42" {
		t.Fatalf("result = %#v", result)
	}
	want := []string{
		"/usr/local/bin/engram",
		"save",
		"Chose typed agent messages",
		"Agents exchange typed messages and artifact references.",
		"--project", "ai-engineering-os",
		"--type", "architecture",
		"--scope", "project",
		"--topic", "architecture/agent-messages",
	}
	if len(runner.calls) != 1 || !reflect.DeepEqual(runner.calls[0], want) {
		t.Fatalf("calls = %#v", runner.calls)
	}
}

func TestRecallUsesExplicitProjectAndBoundedLimit(t *testing.T) {
	runner := &fakeRunner{path: "/usr/local/bin/engram"}
	client := Client{Binary: "engram", Runner: runner}

	result, err := client.Recall(context.Background(), memory.RecallRequest{
		Project: "ai-engineering-os",
		Query:   "quality baseline",
		Limit:   3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Project != "ai-engineering-os" || result.Query != "quality baseline" {
		t.Fatalf("result = %#v", result)
	}
	want := []string{
		"/usr/local/bin/engram",
		"search",
		"quality baseline",
		"--project", "ai-engineering-os",
		"--limit", "3",
	}
	if len(runner.calls) != 1 || !reflect.DeepEqual(runner.calls[0], want) {
		t.Fatalf("calls = %#v", runner.calls)
	}
}

func TestRecallDefaultsToFiveResults(t *testing.T) {
	runner := &fakeRunner{path: "/usr/local/bin/engram"}
	client := Client{Binary: "engram", Runner: runner}

	if _, err := client.Recall(context.Background(), memory.RecallRequest{
		Project: "ai-engineering-os",
		Query:   "current architecture",
	}); err != nil {
		t.Fatal(err)
	}
	call := runner.calls[0]
	if got := call[len(call)-1]; got != "5" {
		t.Fatalf("limit = %q, want 5", got)
	}
}

func TestProjectMemoryRejectsInvalidRequestsBeforeEngramCall(t *testing.T) {
	runner := &fakeRunner{path: "/usr/local/bin/engram"}
	client := Client{Binary: "engram", Runner: runner}

	if _, err := client.Save(context.Background(), memory.SaveRequest{
		Project: "ai-engineering-os",
		Title:   "Missing type",
		Content: "content",
	}); err == nil {
		t.Fatal("expected invalid memory type")
	}
	if _, err := client.Recall(context.Background(), memory.RecallRequest{
		Project: "ai-engineering-os",
		Query:   "query",
		Limit:   100,
	}); err == nil {
		t.Fatal("expected recall limit rejection")
	}
	if len(runner.calls) != 0 {
		t.Fatalf("invalid requests reached Engram: %#v", runner.calls)
	}
}


func TestHealthClassifiesProbeTimeout(t *testing.T) {
	client := Client{
		Binary: "engram",
		Runner: &fakeRunner{path: "/usr/local/bin/engram", block: true},
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

func TestRecallOperationTimeoutIsClassified(t *testing.T) {
	client := Client{
		Binary: "engram",
		Runner: &fakeRunner{path: "/usr/local/bin/engram", block: true},
		OperationTimeout: 20 * time.Millisecond,
	}
	_, err := client.Recall(context.Background(), memory.RecallRequest{
		Project: "project",
		Query: "architecture",
	})
	if err == nil || !strings.Contains(err.Error(), "[timeout]") {
		t.Fatalf("err=%v", err)
	}
}


func TestDeleteVerifiesProjectBeforeDeletingObservation(t *testing.T) {
	runner := &fakeRunner{path: "/usr/local/bin/engram"}
	client := Client{Binary: "engram", Runner: runner}

	result, err := client.Delete(context.Background(), memory.DeleteRequest{
		Project: "ai-engineering-os",
		MemoryID: "42",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Deleted || result.Project != "ai-engineering-os" || result.MemoryID != "42" {
		t.Fatalf("result=%#v", result)
	}
	wantOwnership := []string{
		"/usr/local/bin/engram", "timeline", "42",
		"--before", "0", "--after", "0",
		"--project", "ai-engineering-os",
	}
	wantDelete := []string{"/usr/local/bin/engram", "delete", "42"}
	if len(runner.calls) != 2 ||
		!reflect.DeepEqual(runner.calls[0], wantOwnership) ||
		!reflect.DeepEqual(runner.calls[1], wantDelete) {
		t.Fatalf("calls=%#v", runner.calls)
	}
}

func TestParseSavedMemoryID(t *testing.T) {
	if got := parseSavedMemoryID(`Memory saved: #123 "Architecture" (architecture)`); got != "123" {
		t.Fatalf("got=%q", got)
	}
	if got := parseSavedMemoryID("unexpected output"); got != "" {
		t.Fatalf("got=%q", got)
	}
}
