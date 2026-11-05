package memory

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/artifact"
)

type fakeProjectMemory struct {
	status      Status
	statusErr   error
	recall      RecallResult
	recallErr   error
	saves       []SaveRequest
	saveErrFor  map[string]error
}

func (f *fakeProjectMemory) ID() string { return "fake-memory" }

func (f *fakeProjectMemory) Status(context.Context) (Status, error) {
	return f.status, f.statusErr
}

func (f *fakeProjectMemory) Recall(_ context.Context, request RecallRequest) (RecallResult, error) {
	if f.recallErr != nil {
		return RecallResult{}, f.recallErr
	}
	result := f.recall
	result.Provider = f.ID()
	result.Project = request.Project
	result.Query = request.Query
	return result, nil
}

func (f *fakeProjectMemory) Save(_ context.Context, request SaveRequest) (SaveResult, error) {
	f.saves = append(f.saves, request)
	if err := f.saveErrFor[request.TopicKey]; err != nil {
		return SaveResult{}, err
	}
	return SaveResult{Provider: f.ID(), Project: request.Project, Saved: true}, nil
}

func healthyFakeMemory() *fakeProjectMemory {
	return &fakeProjectMemory{
		status: Status{Provider: "fake-memory", Available: true, Healthy: true},
		saveErrFor: map[string]error{},
	}
}

func TestRecallBestEffortBoundsAndLabelsUntrustedContext(t *testing.T) {
	provider := healthyFakeMemory()
	provider.recall = RecallResult{Context: strings.Repeat("memory ", 2000)}

	got := RecallBestEffort(
		context.Background(),
		provider,
		"project-a",
		"current architecture",
		5,
		1024,
	)
	if !got.Used || got.Warning != "" {
		t.Fatalf("got=%#v", got)
	}
	if len(got.Context) > 1400 {
		t.Fatalf("context unexpectedly large: %d", len(got.Context))
	}
	if !strings.Contains(got.Context, "untrusted historical evidence") {
		t.Fatalf("context missing trust boundary: %q", got.Context)
	}
}

func TestRecallBestEffortDoesNotBlockOnUnavailableProvider(t *testing.T) {
	provider := healthyFakeMemory()
	provider.status = Status{
		Provider: "fake-memory",
		Available: false,
		Healthy: false,
		Details: "offline",
	}
	got := RecallBestEffort(context.Background(), provider, "project", "task", 5, 1024)
	if got.Used || !strings.Contains(got.Warning, "offline") {
		t.Fatalf("got=%#v", got)
	}
}

func TestPersistCuratedArtifactsOnlySavesDurableKnowledge(t *testing.T) {
	provider := healthyFakeMemory()
	now := time.Now().UTC()
	values := []artifact.Artifact{
		mustArtifact(t, artifact.NewRequest{
			RunID: "run-1", TaskID: "a", ProducerAgent: "architect",
			Type: artifact.TypeArchitectureDecision,
			Summary: "Use event sourcing for the control plane",
			Payload: artifact.ArchitectureDecisionPayload{
				Decision: "Use append-only events",
				Rationale: "Auditable state",
			},
		}, now),
		mustArtifact(t, artifact.NewRequest{
			RunID: "run-1", TaskID: "b", ProducerAgent: "researcher",
			Type: artifact.TypeResearchNote,
			Summary: "Gradle emits deterministic failed test identifiers",
			Payload: artifact.ResearchNotePayload{
				SchemaVersion: artifact.LifecyclePayloadSchemaVersion,
				Finding: "Gradle plain console output includes Class > test FAILED",
			},
		}, now.Add(time.Second)),
		mustArtifact(t, artifact.NewRequest{
			RunID: "run-1", TaskID: "c", ProducerAgent: "backend-engineer",
			Type: artifact.TypeHandoff,
			Summary: "Transient handoff",
			Payload: artifact.HandoffPayload{
				SchemaVersion: artifact.LifecyclePayloadSchemaVersion,
				TaskID: "c", AgentID: "backend-engineer", Summary: "done",
			},
		}, now.Add(2*time.Second)),
	}

	report := PersistCuratedArtifactsBestEffort(
		context.Background(), provider, "project-stable", values,
	)
	if report.Saved != 2 || len(provider.saves) != 2 {
		t.Fatalf("report=%#v saves=%#v", report, provider.saves)
	}
	for _, saved := range provider.saves {
		if saved.Project != "project-stable" {
			t.Fatalf("saved project=%q", saved.Project)
		}
		if saved.Type != TypeArchitecture && saved.Type != TypeDiscovery {
			t.Fatalf("unexpected type=%q", saved.Type)
		}
		if strings.TrimSpace(saved.TopicKey) == "" {
			t.Fatalf("missing topic key: %#v", saved)
		}
	}
}

func TestPersistCuratedArtifactsDeduplicatesSameTopic(t *testing.T) {
	provider := healthyFakeMemory()
	now := time.Now().UTC()
	first := mustArtifact(t, artifact.NewRequest{
		RunID: "run-1", TaskID: "a", ProducerAgent: "architect",
		Type: artifact.TypeDecision,
		Summary: "Keep promotion approval explicit",
		Payload: artifact.DecisionPayload{
			SchemaVersion: artifact.LifecyclePayloadSchemaVersion,
			Decision: "Require explicit approval",
			Rationale: "Safety",
		},
	}, now)
	second := first
	second.ID = strings.Replace(first.ID, "artifact-", "artifact-a", 1)
	second.TaskID = "b"
	second.CreatedAt = now.Add(time.Second)
	if err := artifact.Validate(second); err != nil {
		// Build a proper second artifact if ID mutation no longer satisfies the ID contract.
		second = mustArtifact(t, artifact.NewRequest{
			RunID: "run-1", TaskID: "b", ProducerAgent: "reviewer",
			Type: artifact.TypeDecision,
			Summary: "Keep promotion approval explicit",
			Payload: artifact.DecisionPayload{
				SchemaVersion: artifact.LifecyclePayloadSchemaVersion,
				Decision: "Require explicit approval",
				Rationale: "Safety",
			},
		}, now.Add(time.Second))
	}

	report := PersistCuratedArtifactsBestEffort(
		context.Background(), provider, "project", []artifact.Artifact{first, second},
	)
	if report.Saved != 1 || len(provider.saves) != 1 {
		t.Fatalf("report=%#v saves=%#v", report, provider.saves)
	}
}

func TestPersistRunLearningSavesBoundedSingleProjectTopic(t *testing.T) {
	provider := healthyFakeMemory()
	report := PersistRunLearningBestEffort(context.Background(), provider, "project", RunLearning{
		Task:   "Add checkout validation",
		Intent: "implementation",
		TaskSummaries: []TaskLearning{
			{TaskID: "backend", AgentID: "backend-engineer", Summary: "Added validation at service boundary."},
			{TaskID: "qa", AgentID: "qa-engineer", Summary: "Regression tests cover invalid payloads."},
		},
	})
	if report.Saved != 1 || len(provider.saves) != 1 {
		t.Fatalf("report=%#v saves=%#v", report, provider.saves)
	}
	saved := provider.saves[0]
	if saved.Type != TypeLearning || saved.Project != "project" ||
		!strings.HasPrefix(saved.TopicKey, "run-learning/") {
		t.Fatalf("saved=%#v", saved)
	}
	if strings.Contains(saved.Content, "prompt") {
		t.Fatalf("unexpected transcript-like content=%q", saved.Content)
	}
}

func TestMemoryLifecycleReportsSaveFailureWithoutBlocking(t *testing.T) {
	provider := healthyFakeMemory()
	provider.saveErrFor["run-learning/ignored"] = errors.New("provider down")
	// Directly verify best-effort behavior with an unhealthy provider status.
	provider.statusErr = errors.New("doctor failed")
	report := PersistRunLearningBestEffort(context.Background(), provider, "project", RunLearning{Task: "x"})
	if report.Saved != 0 || len(report.Warnings) == 0 {
		t.Fatalf("report=%#v", report)
	}
}

func mustArtifact(t *testing.T, request artifact.NewRequest, now time.Time) artifact.Artifact {
	t.Helper()
	value, err := artifact.New(request, now)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
