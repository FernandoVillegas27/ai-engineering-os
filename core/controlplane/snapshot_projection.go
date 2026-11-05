package controlplane

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/FernandoVillegas27/ai-engineering-os/core/fslock"
)

// ProjectFromState continues projection from a previously validated state.
// The supplied events must start exactly at base.LastSequence+1.
func ProjectFromState(base State, events []Event) (State, error) {
	if strings.TrimSpace(base.RunID) == "" {
		return State{}, errors.New("control-plane base projection requires run id")
	}
	if base.LastSequence == 0 {
		return State{}, errors.New("control-plane base projection requires a non-zero sequence")
	}
	if err := validateSnapshotDerivedState(base); err != nil {
		return State{}, fmt.Errorf("invalid base projection state: %w", err)
	}

	state := cloneProjectionState(base)
	for i, event := range events {
		expected := base.LastSequence + uint64(i) + 1
		if event.Sequence != expected {
			return State{}, fmt.Errorf(
				"control-plane continuation sequence mismatch: got %d want %d",
				event.Sequence,
				expected,
			)
		}
		if event.RunID != state.RunID {
			return State{}, errors.New("control-plane continuation cannot mix run ids")
		}
		if err := ValidateEvent(event); err != nil {
			return State{}, fmt.Errorf("invalid control-plane event %d: %w", event.Sequence, err)
		}
		if event.Type == EventRunStarted {
			return State{}, errors.New("control-plane continuation cannot contain run.started")
		}
		if terminalRun(state.Status) && !allowedAfterTerminal(event.Type) {
			return State{}, fmt.Errorf("event %s appears after terminal run state", event.Type)
		}
		if err := applyProjectionEvent(&state, event); err != nil {
			return State{}, err
		}
		state.LastSequence = event.Sequence
	}
	state.recompute()
	return state, nil
}

// ProjectVerified uses the latest immutable snapshot when available. Historical
// event bytes covered by the snapshot are still verified before and after the
// suffix projection; no event files are deleted or trusted blindly.
func (s *SnapshotStore) ProjectVerified(
	ctx context.Context,
	events *EventStore,
	runID string,
) (State, bool, error) {
	if s == nil {
		return State{}, false, errors.New("control-plane snapshot store is required")
	}
	if events == nil {
		return State{}, false, errors.New("control-plane event store is required")
	}
	if !safeToken(runID) {
		return State{}, false, errors.New("run id is unsafe for control-plane projection")
	}

	record, err := s.Latest(runID)
	if os.IsNotExist(err) {
		state, fullErr := projectFullLocked(ctx, events, runID)
		return state, false, fullErr
	}
	if err != nil {
		return State{}, false, err
	}
	if err := ValidateSnapshot(record); err != nil {
		return State{}, false, err
	}

	runDir := filepath.Join(events.root, runID)
	info, err := os.Lstat(runDir)
	if err != nil {
		return State{}, false, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return State{}, false, errors.New("control-plane run directory must be a regular directory")
	}

	lock, err := fslock.Acquire(ctx, filepath.Join(runDir, ".append.lock"), fslock.Options{})
	if err != nil {
		return State{}, false, fmt.Errorf("acquire control-plane projection lock: %w", err)
	}
	defer lock.Release()

	before, err := rawEventPrefixDigest(runDir, record.ThroughSequence)
	if err != nil {
		return State{}, false, err
	}
	if before != record.PrefixDigest {
		return State{}, false, errors.New("control-plane snapshot event prefix digest mismatch")
	}

	suffix, err := loadEventsAfter(runDir, record.ThroughSequence)
	if err != nil {
		return State{}, false, err
	}
	state, err := ProjectFromState(record.State, suffix)
	if err != nil {
		return State{}, false, err
	}

	after, err := rawEventPrefixDigest(runDir, record.ThroughSequence)
	if err != nil {
		return State{}, false, err
	}
	if after != record.PrefixDigest || after != before {
		return State{}, false, errors.New("control-plane event prefix changed during snapshot projection")
	}
	return state, true, nil
}

func projectFullLocked(
	ctx context.Context,
	events *EventStore,
	runID string,
) (State, error) {
	runDir := filepath.Join(events.root, runID)
	info, err := os.Lstat(runDir)
	if err != nil {
		return State{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return State{}, errors.New("control-plane run directory must be a regular directory")
	}
	lock, err := fslock.Acquire(ctx, filepath.Join(runDir, ".append.lock"), fslock.Options{})
	if err != nil {
		return State{}, fmt.Errorf("acquire control-plane projection lock: %w", err)
	}
	defer lock.Release()
	values, err := loadEvents(runDir)
	if err != nil {
		return State{}, err
	}
	if len(values) == 0 {
		return State{}, errors.New("control-plane projection requires at least one event")
	}
	return Project(values)
}

func loadEventsAfter(runDir string, after uint64) ([]Event, error) {
	entries, err := os.ReadDir(runDir)
	if err != nil {
		return nil, err
	}
	type candidate struct {
		sequence uint64
		name     string
	}
	var suffix []candidate
	for _, entry := range entries {
		name := entry.Name()
		if name == ".append.lock" || entry.IsDir() || filepath.Ext(name) != ".json" {
			continue
		}
		sequence := filenameSequence(name)
		if sequence == 0 {
			return nil, fmt.Errorf("control-plane event filename has invalid sequence: %s", name)
		}
		if sequence <= after {
			continue
		}
		suffix = append(suffix, candidate{sequence: sequence, name: name})
	}
	sort.Slice(suffix, func(i, j int) bool {
		if suffix[i].sequence != suffix[j].sequence {
			return suffix[i].sequence < suffix[j].sequence
		}
		return suffix[i].name < suffix[j].name
	})

	values := make([]Event, 0, len(suffix))
	for i, candidate := range suffix {
		expected := after + uint64(i) + 1
		if candidate.sequence != expected {
			return nil, fmt.Errorf(
				"control-plane event suffix sequence gap or duplicate: got %d want %d",
				candidate.sequence,
				expected,
			)
		}
		path := filepath.Join(runDir, candidate.name)
		info, err := os.Lstat(path)
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return nil, fmt.Errorf("control-plane event entry is not regular: %s", candidate.name)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var value Event
		decoder := json.NewDecoder(strings.NewReader(string(data)))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&value); err != nil {
			return nil, fmt.Errorf("decode control-plane event %s: %w", candidate.name, err)
		}
		if err := ValidateEvent(value); err != nil {
			return nil, fmt.Errorf("validate control-plane event %s: %w", candidate.name, err)
		}
		if value.Sequence != candidate.sequence {
			return nil, fmt.Errorf("control-plane event filename sequence mismatch: %s", candidate.name)
		}
		values = append(values, value)
	}
	return values, nil
}

func cloneProjectionState(state State) State {
	clone := state
	clone.Tasks = make(map[string]TaskState, len(state.Tasks))
	for id, task := range state.Tasks {
		task.DependsOn = append([]string(nil), task.DependsOn...)
		task.Scopes = append([]string(nil), task.Scopes...)
		clone.Tasks[id] = task
	}
	clone.Agents = make(map[string]AgentState, len(state.Agents))
	for id, agent := range state.Agents {
		agent.ActiveTasks = append([]string(nil), agent.ActiveTasks...)
		clone.Agents[id] = agent
	}
	clone.Edges = append([]TaskEdge(nil), state.Edges...)
	return clone
}
