package controlplane

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/artifact"
	"github.com/FernandoVillegas27/ai-engineering-os/core/message"
)

type HTTPServer struct {
	Store        *EventStore
	Artifacts    *artifact.Store
	Messages     *message.Bus
	PollInterval time.Duration
}

func (s HTTPServer) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/runs", s.handleRunIndex)
	mux.HandleFunc("/runs/", s.handleRuns)
	return securityHeaders(mux)
}

func (s HTTPServer) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type RunSummary struct {
	RunID            string    `json:"run_id"`
	Status           RunStatus `json:"status"`
	LastSequence     uint64    `json:"last_sequence"`
	Progress         Progress  `json:"progress"`
	AgentCount       int       `json:"agent_count"`
	MessageCount     int       `json:"message_count"`
	ArtifactCount    int       `json:"artifact_count"`
	PendingApprovals int       `json:"pending_approvals"`
	LastEventAt      time.Time `json:"last_event_at"`
}

func (s HTTPServer) handleRunIndex(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if s.Store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "control-plane event store is not configured"})
		return
	}
	ids, err := s.Store.RunIDs()
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	summaries := make([]RunSummary, 0, len(ids))
	for _, runID := range ids {
		events, err := s.Store.List(runID)
		if err != nil {
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}
		if len(events) == 0 {
			continue
		}
		state, err := Project(events)
		if err != nil {
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}
		summaries = append(summaries, RunSummary{
			RunID:            runID,
			Status:           state.Status,
			LastSequence:     state.LastSequence,
			Progress:         state.Progress,
			AgentCount:       len(state.Agents),
			MessageCount:     state.MessageCount,
			ArtifactCount:    state.ArtifactCount,
			PendingApprovals: state.PendingApprovals,
			LastEventAt:      events[len(events)-1].CreatedAt,
		})
	}
	sort.Slice(summaries, func(i, j int) bool {
		if !summaries[i].LastEventAt.Equal(summaries[j].LastEventAt) {
			return summaries[i].LastEventAt.After(summaries[j].LastEventAt)
		}
		return summaries[i].RunID < summaries[j].RunID
	})
	writeJSON(w, http.StatusOK, summaries)
}

func (s HTTPServer) handleRuns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if s.Store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "control-plane event store is not configured"})
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/runs/"), "/")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	runID := parts[0]
	resource := parts[1]

	switch resource {
	case "state":
		s.handleState(w, r, runID)
	case "events":
		s.handleEvents(w, r, runID)
	case "stream":
		s.handleStream(w, r, runID)
	case "messages":
		s.handleMessages(w, r, runID)
	case "artifacts":
		s.handleArtifacts(w, r, runID)
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	}
}

func (s HTTPServer) handleState(w http.ResponseWriter, _ *http.Request, runID string) {
	events, err := s.Store.List(runID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if len(events) == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "run not found"})
		return
	}
	state, err := Project(events)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (s HTTPServer) handleEvents(w http.ResponseWriter, _ *http.Request, runID string) {
	events, err := s.Store.List(runID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if len(events) == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "run not found"})
		return
	}
	writeJSON(w, http.StatusOK, events)
}

func (s HTTPServer) handleMessages(w http.ResponseWriter, _ *http.Request, runID string) {
	if s.Messages == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "message bus is not configured"})
		return
	}
	values, err := s.Messages.List(runID, true, time.Time{})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, values)
}

func (s HTTPServer) handleArtifacts(w http.ResponseWriter, _ *http.Request, runID string) {
	if s.Artifacts == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "artifact store is not configured"})
		return
	}
	values, err := s.Artifacts.ListRun(runID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, values)
}

func (s HTTPServer) handleStream(w http.ResponseWriter, r *http.Request, runID string) {
	after, err := parseAfter(r.URL.Query().Get("after"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming is not supported"})
		return
	}

	initial, err := s.Store.List(runID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if len(initial) == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "run not found"})
		return
	}

	interval := s.PollInterval
	if interval <= 0 {
		interval = 500 * time.Millisecond
	}
	if interval < 100*time.Millisecond {
		interval = 100 * time.Millisecond
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	last := after
	send := func(ctx context.Context) error {
		events, err := s.Store.List(runID)
		if err != nil {
			return err
		}
		for _, event := range events {
			if event.Sequence <= last {
				continue
			}
			if err := writeSSEEvent(w, event); err != nil {
				return err
			}
			last = event.Sequence
			flusher.Flush()
		}
		return ctx.Err()
	}

	_ = send(r.Context())
	if r.Context().Err() != nil {
		return
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			if err := send(r.Context()); err != nil && r.Context().Err() == nil {
				_, _ = fmt.Fprintf(w, "event: error\ndata: %q\n\n", err.Error())
				flusher.Flush()
				return
			}
		}
	}
}

func parseAfter(value string) (uint64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	after, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("after must be an unsigned sequence number")
	}
	return after, nil
}

func writeSSEEvent(w http.ResponseWriter, event Event) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "id: %d\n", event.Sequence); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "event: %s\n", event.Type); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
		return err
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}
