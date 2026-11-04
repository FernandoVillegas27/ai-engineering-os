package controlplane

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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
	if strings.Contains(body, "id: 1
") {
		t.Fatalf("stream replayed already acknowledged event: %q", body)
	}
	if !strings.Contains(body, "id: 2
") || !strings.Contains(body, "id: 3
") {
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
