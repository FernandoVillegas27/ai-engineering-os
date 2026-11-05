package controlplane

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/artifact"
	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/message"
	"github.com/FernandoVillegas27/ai-engineering-os/core/promotion"
	"github.com/FernandoVillegas27/ai-engineering-os/core/promotioncandidate"
	"github.com/FernandoVillegas27/ai-engineering-os/core/staging"
)

func testHTTPStore(t *testing.T) (*EventStore, string) {
	t.Helper()
	store, err := NewEventStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runID := "run-http"
	for _, typ := range []EventType{EventRunStarted, EventTaskStarted, EventTaskCompleted} {
		req := NewEventRequest{
			RunID: runID,
			Type:  typ,
			Data:  map[string]bool{"ok": true},
		}
		if typ == EventTaskStarted || typ == EventTaskCompleted {
			req.TaskID = "task-1"
			req.AgentID = "backend-engineer"
		}
		value, err := NewEvent(req, time.Now().UTC())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.Append(context.Background(), value); err != nil {
			t.Fatal(err)
		}
	}
	return store, runID
}

func TestHTTPStateAndEventsEndpoints(t *testing.T) {
	store, runID := testHTTPStore(t)
	handler := (HTTPServer{Store: store}).Handler()

	stateReq := httptest.NewRequest(http.MethodGet, "/runs/"+runID+"/state", nil)
	stateRec := httptest.NewRecorder()
	handler.ServeHTTP(stateRec, stateReq)
	if stateRec.Code != http.StatusOK {
		t.Fatalf("state status=%d body=%s", stateRec.Code, stateRec.Body.String())
	}
	var state State
	if err := json.Unmarshal(stateRec.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	if state.RunID != runID || state.Progress.Completed != 1 {
		t.Fatalf("state=%#v", state)
	}
	if stateRec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("missing security headers: %#v", stateRec.Header())
	}

	eventsReq := httptest.NewRequest(http.MethodGet, "/runs/"+runID+"/events", nil)
	eventsRec := httptest.NewRecorder()
	handler.ServeHTTP(eventsRec, eventsReq)
	if eventsRec.Code != http.StatusOK {
		t.Fatalf("events status=%d body=%s", eventsRec.Code, eventsRec.Body.String())
	}
	var events []Event
	if err := json.Unmarshal(eventsRec.Body.Bytes(), &events); err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("events=%#v", events)
	}
}

func TestHTTPRejectsUnknownRunsAndNonGetMethods(t *testing.T) {
	store, _ := testHTTPStore(t)
	handler := (HTTPServer{Store: store}).Handler()

	req := httptest.NewRequest(http.MethodGet, "/runs/missing/state", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/runs/run-http/state", strings.NewReader("{}"))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed || rec.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("status=%d headers=%#v", rec.Code, rec.Header())
	}
}

func TestHTTPStreamCanResumeAfterSequence(t *testing.T) {
	store, runID := testHTTPStore(t)
	handler := (HTTPServer{Store: store, PollInterval: time.Second}).Handler()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest(http.MethodGet, "/runs/"+runID+"/stream?after=1", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, "id: 1\n") {
		t.Fatalf("stream replayed already acknowledged event: %q", body)
	}
	if !strings.Contains(body, "id: 2\n") || !strings.Contains(body, "id: 3\n") {
		t.Fatalf("stream did not resume correctly: %q", body)
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "text/event-stream") {
		t.Fatalf("content type=%q", rec.Header().Get("Content-Type"))
	}
}

func TestHTTPStreamRejectsInvalidAfter(t *testing.T) {
	store, runID := testHTTPStore(t)
	handler := (HTTPServer{Store: store}).Handler()

	req := httptest.NewRequest(http.MethodGet, "/runs/"+runID+"/stream?after=abc", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}


func TestHTTPRunIndexReturnsRecentProjectedSummaries(t *testing.T) {
	store, err := NewEventStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 4, 22, 0, 0, 0, time.UTC)

	appendForRun := func(runID string, typ EventType, at time.Time) {
		t.Helper()
		req := NewEventRequest{RunID: runID, Type: typ, Data: map[string]bool{"ok": true}}
		if typ == EventTaskStarted || typ == EventTaskCompleted {
			req.TaskID = "task-1"
			req.AgentID = "backend-engineer"
		}
		value, newErr := NewEvent(req, at)
		if newErr != nil {
			t.Fatal(newErr)
		}
		if _, appendErr := store.Append(context.Background(), value); appendErr != nil {
			t.Fatal(appendErr)
		}
	}
	appendForRun("run-old", EventRunStarted, now)
	appendForRun("run-old", EventTaskCompleted, now.Add(time.Minute))
	appendForRun("run-new", EventRunStarted, now.Add(2*time.Minute))
	appendForRun("run-new", EventTaskStarted, now.Add(3*time.Minute))

	req := httptest.NewRequest(http.MethodGet, "/runs", nil)
	rec := httptest.NewRecorder()
	(HTTPServer{Store: store}).Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var summaries []RunSummary
	if err := json.Unmarshal(rec.Body.Bytes(), &summaries); err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 2 || summaries[0].RunID != "run-new" || summaries[1].RunID != "run-old" {
		t.Fatalf("summaries=%#v", summaries)
	}
	if summaries[0].Progress.Running != 1 || summaries[0].AgentCount != 1 {
		t.Fatalf("new summary=%#v", summaries[0])
	}
	if summaries[1].Progress.Completed != 1 {
		t.Fatalf("old summary=%#v", summaries[1])
	}
}

func TestHTTPRunIndexIsGetOnly(t *testing.T) {
	store, _ := NewEventStore(t.TempDir())
	req := httptest.NewRequest(http.MethodPost, "/runs", nil)
	rec := httptest.NewRecorder()
	(HTTPServer{Store: store}).Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed || rec.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("status=%d headers=%#v", rec.Code, rec.Header())
	}
}


func TestHTTPCollaborationEndpointsReturnRunScopedData(t *testing.T) {
	eventStore, runID := testHTTPStore(t)
	artifactStore, err := artifact.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	messageBus, err := message.NewBus(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	value, err := artifact.New(artifact.NewRequest{
		RunID: runID, TaskID: "task-1", ProducerAgent: "architect",
		Type: artifact.TypeHandoff, Summary: "handoff", Payload: map[string]string{"ok": "true"},
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := artifactStore.Save(value); err != nil {
		t.Fatal(err)
	}
	msg, err := message.New(message.NewRequest{
		RunID: runID, TaskID: "task-1", FromAgent: "architect",
		To: message.Target{Kind: message.TargetAgent, ID: "backend-engineer"},
		Kind: message.KindHandoff, ArtifactRefs: []string{value.ID},
		Payload: map[string]string{"summary": "handoff"},
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := messageBus.Append(context.Background(), msg); err != nil {
		t.Fatal(err)
	}
	handler := (HTTPServer{Store: eventStore, Artifacts: artifactStore, Messages: messageBus}).Handler()

	for _, path := range []string{"/runs/" + runID + "/messages", "/runs/" + runID + "/artifacts"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", path, rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), runID) {
			t.Fatalf("%s body=%s", path, rec.Body.String())
		}
	}
}

func TestHTTPCollaborationEndpointsRequireConfiguredStores(t *testing.T) {
	eventStore, runID := testHTTPStore(t)
	handler := (HTTPServer{Store: eventStore}).Handler()
	for _, path := range []string{"/runs/" + runID + "/messages", "/runs/" + runID + "/artifacts"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s status=%d", path, rec.Code)
		}
	}
}


func TestHTTPPromotionStatusReportsCurrentDryRunReadiness(t *testing.T) {
	eventStore, runID := testHTTPStore(t)
	base := t.TempDir()
	original := filepath.Join(base, "original")
	staged := filepath.Join(base, "staged")
	if err := os.MkdirAll(original, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(staged, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(original, "main.go"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staged, "main.go"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := staging.SnapshotWorkspace(original, execution.DefaultSafeViewPolicy())
	if err != nil {
		t.Fatal(err)
	}
	request := promotion.Request{
		OriginalRoot: original, StagedWorkspace: staged,
		PreImage: staging.NewPreImage(before),
		Declared: []staging.WorkspaceChange{{Path: "main.go", Action: "modify"}},
		QualityPassed: true, TempParent: base,
	}
	record, err := promotioncandidate.New(runID, request, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	store, err := promotioncandidate.NewStore(filepath.Join(base, "candidates"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(record); err != nil {
		t.Fatal(err)
	}
	handler := (HTTPServer{Store: eventStore, Promotions: store}).Handler()

	req := httptest.NewRequest(http.MethodGet, "/runs/"+runID+"/promotion", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var status PromotionStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if !status.Ready || !status.QualityPassed || status.ChangeCount != 1 {
		t.Fatalf("status=%#v", status)
	}

	// Stale the candidate by changing the original. The endpoint remains
	// read-only and reports ready=false without leaking the integrity error.
	if err := os.WriteFile(filepath.Join(original, "main.go"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.Ready {
		t.Fatalf("stale promotion reported ready: %#v", status)
	}
}

func TestHTTPPromotionStatusReturns404WithoutCandidate(t *testing.T) {
	eventStore, runID := testHTTPStore(t)
	store, err := promotioncandidate.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/runs/"+runID+"/promotion", nil)
	rec := httptest.NewRecorder()
	(HTTPServer{Store: eventStore, Promotions: store}).Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}


func TestHTTPRunControlsUsesInjectedDeterministicProvider(t *testing.T) {
	store, runID := testHTTPStore(t)
	handler := (HTTPServer{
		Store: store,
		Controls: func(context.Context, string) (RunControls, error) {
			return RunControls{
				Consistent: true,
				CanPause: true,
				CanCancel: true,
				PauseBoundary: "task-boundary",
			}, nil
		},
	}).Handler()
	req := httptest.NewRequest(http.MethodGet, "/runs/"+runID+"/controls", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var controls RunControls
	if err := json.Unmarshal(rec.Body.Bytes(), &controls); err != nil {
		t.Fatal(err)
	}
	if !controls.Consistent || !controls.CanPause || !controls.CanCancel || controls.PauseBoundary != "task-boundary" {
		t.Fatalf("controls=%#v", controls)
	}
}

func TestHTTPRunControlsRequiresProvider(t *testing.T) {
	store, runID := testHTTPStore(t)
	req := httptest.NewRequest(http.MethodGet, "/runs/"+runID+"/controls", nil)
	rec := httptest.NewRecorder()
	(HTTPServer{Store: store}).Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}
