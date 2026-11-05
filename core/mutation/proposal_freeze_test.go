package mutation

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/orchestration"
	"github.com/FernandoVillegas27/ai-engineering-os/core/proposal"
)

type fakeProposalRuntime struct {
	id     string
	result proposal.Proposal
	err    error
}

func (f fakeProposalRuntime) ID() string { return f.id }

func (f fakeProposalRuntime) Propose(context.Context, execution.WorkUnit) (proposal.Proposal, error) {
	return f.result, f.err
}

func proposalUnit(id, agent, task string) execution.WorkUnit {
	return execution.WorkUnit{
		ID:          id,
		Task:        task,
		AgentID:     agent,
		Intent:      orchestration.IntentResearch,
		ProjectRoot: ".",
		Mode:        execution.ModeReadOnly,
		Permissions: execution.PermissionEnvelope{
			SourceCode: "read",
			Tests:      "read",
			GitCommit:  "deny",
			GitPush:    "deny",
			Secrets:    "deny",
			Production: "deny",
		},
	}
}

func validProposal(summary, path, action string) proposal.Proposal {
	return proposal.Proposal{
		Summary: summary,
		Files: []proposal.FileChange{{
			Path:      path,
			Action:    action,
			Rationale: summary,
		}},
		Risks:        []proposal.Risk{},
		Verification: []string{summary},
	}
}

func TestFreezeProposalsValidatesAndOrdersOwnership(t *testing.T) {
	units := []execution.WorkUnit{
		proposalUnit("work-b", "backend-engineer", "backend change"),
		proposalUnit("work-a", "frontend-engineer", "frontend change"),
	}
	runtimes := map[string]proposal.Runtime{
		"backend-engineer": fakeProposalRuntime{
			id: "backend", result: validProposal("backend change", "backend/api.go", "create"),
		},
		"frontend-engineer": fakeProposalRuntime{
			id: "frontend", result: validProposal("frontend change", "frontend/app.ts", "create"),
		},
	}
	frozen, err := FreezeProposals(context.Background(), units, func(unit execution.WorkUnit) (proposal.Runtime, error) {
		return runtimes[unit.AgentID], nil
	}, FreezeConfig{MaxConcurrency: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(frozen) != 2 || frozen[0].TaskID != "work-a" || frozen[1].TaskID != "work-b" {
		t.Fatalf("frozen = %#v", frozen)
	}
	if frozen[0].Declared[0].Path != "frontend/app.ts" ||
		frozen[1].Declared[0].Path != "backend/api.go" {
		t.Fatalf("declared = %#v", frozen)
	}

	// Returned proposals are copies; mutating the original runtime result after
	// freeze cannot change the frozen file list.
	original := runtimes["backend-engineer"].(fakeProposalRuntime)
	original.result.Files[0].Path = "other.go"
	if frozen[1].Proposal.Files[0].Path != "backend/api.go" {
		t.Fatalf("frozen proposal mutated: %#v", frozen[1].Proposal)
	}
}

func TestFreezeProposalsRejectsExactFileOwnershipConflict(t *testing.T) {
	units := []execution.WorkUnit{
		proposalUnit("backend", "backend-engineer", "shared backend"),
		proposalUnit("frontend", "frontend-engineer", "shared frontend"),
	}
	frozen, err := FreezeProposals(context.Background(), units, func(unit execution.WorkUnit) (proposal.Runtime, error) {
		return fakeProposalRuntime{
			id: unit.AgentID,
			result: validProposal(unit.Task, "shared.go", "modify"),
		}, nil
	}, FreezeConfig{MaxConcurrency: 2})
	if err == nil || !strings.Contains(err.Error(), "ownership conflict") {
		t.Fatalf("frozen=%#v err=%#v", frozen, err)
	}
}

func TestFreezeProposalsRejectsEmptyDeclaredScope(t *testing.T) {
	unit := proposalUnit("architect", "architect", "architecture change")
	_, err := FreezeProposals(context.Background(), []execution.WorkUnit{unit}, func(execution.WorkUnit) (proposal.Runtime, error) {
		return fakeProposalRuntime{
			id: "architect",
			result: proposal.Proposal{
				Summary:      "architecture change",
				Files:        nil,
				Risks:        []proposal.Risk{},
				Verification: []string{"architecture change"},
			},
		}, nil
	}, FreezeConfig{})
	if err == nil || !strings.Contains(err.Error(), "declares no files") {
		t.Fatalf("err = %#v", err)
	}
}

func TestFreezeProposalsPropagatesRuntimeResolutionAndGenerationErrors(t *testing.T) {
	unit := proposalUnit("backend", "backend-engineer", "backend change")
	sentinel := errors.New("runtime unavailable")
	_, err := FreezeProposals(context.Background(), []execution.WorkUnit{unit}, func(execution.WorkUnit) (proposal.Runtime, error) {
		return nil, sentinel
	}, FreezeConfig{})
	if err == nil || !errors.Is(err, sentinel) {
		t.Fatalf("err = %#v", err)
	}

	_, err = FreezeProposals(context.Background(), []execution.WorkUnit{unit}, func(execution.WorkUnit) (proposal.Runtime, error) {
		return fakeProposalRuntime{id: "backend", err: sentinel}, nil
	}, FreezeConfig{})
	if err == nil || !errors.Is(err, sentinel) {
		t.Fatalf("err = %#v", err)
	}
}

func TestFreezeProposalsRejectsDuplicateWorkUnitID(t *testing.T) {
	units := []execution.WorkUnit{
		proposalUnit("same", "backend-engineer", "backend change"),
		proposalUnit("same", "frontend-engineer", "frontend change"),
	}
	_, err := FreezeProposals(context.Background(), units, func(execution.WorkUnit) (proposal.Runtime, error) {
		t.Fatal("resolver must not be called")
		return nil, nil
	}, FreezeConfig{})
	if err == nil {
		t.Fatal("expected duplicate work-unit rejection")
	}
}
