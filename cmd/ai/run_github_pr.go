package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/controlplane"
	"github.com/FernandoVillegas27/ai-engineering-os/core/toolapproval"
	githubintegration "github.com/FernandoVillegas27/ai-engineering-os/integrations/githubcli"
)

type githubPROptions struct {
	runID       string
	taskID      string
	repository string
	head        string
	base        string
	title       string
	body        string
	stateDir    string
	eventsDir   string
	approvalDir string
	json        bool
}

type githubPRCreator interface {
	CreatePullRequest(context.Context, githubintegration.CreatePullRequestRequest, toolapproval.Evidence) (githubintegration.CreatePullRequestResult, error)
}

var newGitHubPRCreator = func() githubPRCreator {
	client := githubintegration.New()
	return client
}

type githubPRView struct {
	RunID         string `json:"run_id"`
	TaskID        string `json:"task_id"`
	Repository    string `json:"repository"`
	Head          string `json:"head"`
	Base          string `json:"base"`
	Title         string `json:"title"`
	RequestDigest string `json:"request_digest"`
	Status        string `json:"status"`
	URL           string `json:"url,omitempty"`
}

func runRunGitHubPR(args []string) error {
	if len(args) == 0 {
		return errors.New("uso: ai run github-pr <prepare|create> ...")
	}
	switch args[0] {
	case "prepare":
		return runRunGitHubPRPrepare(args[1:])
	case "create":
		return runRunGitHubPRCreate(args[1:])
	default:
		return fmt.Errorf("subcomando github-pr desconocido: %s", args[0])
	}
}

func runRunGitHubPRPrepare(args []string) error {
	options, err := parseGitHubPROptions(args, false)
	if err != nil {
		return err
	}
	run, _, err := validateToolApprovalTarget(runToolApprovalOptions{
		runID: options.runID, taskID: options.taskID,
		toolID: "github", operation: "create-pull-request",
		stateDir: options.stateDir,
	})
	if err != nil {
		return err
	}
	request := githubPRRequest(options, run.ProjectRoot)
	digest, err := request.RequestDigest()
	if err != nil {
		return err
	}
	approvalOptions := runToolApprovalOptions{
		runID: options.runID,
		taskID: options.taskID,
		toolID: "github",
		operation: "create-pull-request",
		requestDigest: digest,
		stateDir: options.stateDir,
		eventsDir: options.eventsDir,
	}
	events, eventStore, err := loadToolApprovalEvents(approvalOptions)
	if err != nil {
		return err
	}
	if toolApprovalPending(events, options.taskID, "github", "create-pull-request", digest) {
		return errors.New("approval is already pending for this exact pull-request request")
	}
	approvalEvent, err := controlplane.NewEvent(controlplane.NewEventRequest{
		RunID: options.runID,
		TaskID: options.taskID,
		Type: controlplane.EventApprovalRequested,
		Summary: "github/create-pull-request requires human approval",
		Data: map[string]any{
			"kind": "tool-operation",
			"tool_id": "github",
			"operation": "create-pull-request",
			"request_digest": digest,
		},
	}, time.Now().UTC())
	if err != nil {
		return err
	}
	if _, err := eventStore.Append(context.Background(), approvalEvent); err != nil {
		return err
	}
	return printGitHubPRView(githubPRView{
		RunID: options.runID, TaskID: options.taskID,
		Repository: options.repository, Head: options.head, Base: options.base,
		Title: options.title, RequestDigest: digest, Status: "pending-approval",
	}, options.json)
}

func runRunGitHubPRCreate(args []string) error {
	options, err := parseGitHubPROptions(args, true)
	if err != nil {
		return err
	}
	run, _, err := validateToolApprovalTarget(runToolApprovalOptions{
		runID: options.runID, taskID: options.taskID,
		toolID: "github", operation: "create-pull-request",
		stateDir: options.stateDir,
	})
	if err != nil {
		return err
	}
	request := githubPRRequest(options, run.ProjectRoot)
	digest, err := request.RequestDigest()
	if err != nil {
		return err
	}
	approvalStore, err := openToolApprovalStore(options.approvalDir)
	if err != nil {
		return err
	}
	approvals, err := approvalStore.List(options.runID)
	if err != nil {
		return err
	}
	approval, ok := findPullRequestApproval(
		approvals, options.runID, options.taskID, digest, time.Now().UTC(),
	)
	if !ok {
		return errors.New("no matching non-expired human approval for this exact pull-request request")
	}
	eventStore, err := openControlPlaneStore(options.eventsDir)
	if err != nil {
		return err
	}
	events, err := eventStore.List(options.runID)
	if err != nil {
		return fmt.Errorf("load approval audit trail: %w", err)
	}
	if !githubPRApprovalRecorded(events, options.taskID, digest, approval.ID) {
		return errors.New("matching approval evidence has no resolved human grant in the Control Plane audit trail")
	}
	// The external operation is not safe to replay after its start was
	// recorded: a process crash can happen after GitHub accepted the request
	// but before the completion event was durably appended.
	if githubPRMutationStarted(events, options.taskID, digest) {
		return errors.New("this pull-request mutation has already started; inspect GitHub and the audit trail before any new approval")
	}
	// Claim the exact side effect before writing the pre-mutation event. The
	// append-only audit check above prevents sequential replay, while this
	// O_EXCL claim prevents concurrent processes from both passing that check.
	// A failed audit append leaves the claim intact to fail closed.
	if err := approvalStore.ClaimMutation(
		options.runID, options.taskID, "github", "create-pull-request", digest,
	); err != nil {
		return fmt.Errorf("reserve github/create-pull-request mutation: %w", err)
	}

	started, err := controlplane.NewEvent(controlplane.NewEventRequest{
		RunID: options.runID,
		TaskID: options.taskID,
		Type: controlplane.EventToolStarted,
		Summary: "github/create-pull-request started",
		Data: map[string]any{
			"tool_id": "github",
			"operation": "create-pull-request",
			"request_digest": digest,
			"approval_id": approval.ID,
		},
	}, time.Now().UTC())
	if err != nil {
		return err
	}
	if _, err := eventStore.Append(context.Background(), started); err != nil {
		return fmt.Errorf("persist pre-mutation audit event: %w", err)
	}

	client := newGitHubPRCreator()
	result, mutationErr := client.CreatePullRequest(context.Background(), request, approval)
	if mutationErr != nil {
		failed, eventErr := controlplane.NewEvent(controlplane.NewEventRequest{
			RunID: options.runID,
			TaskID: options.taskID,
			Type: controlplane.EventToolFailed,
			Summary: "github/create-pull-request failed",
			Data: map[string]any{
				"tool_id": "github",
				"operation": "create-pull-request",
				"request_digest": digest,
				"approval_id": approval.ID,
			},
		}, time.Now().UTC())
		if eventErr == nil {
			_, eventErr = eventStore.Append(context.Background(), failed)
		}
		if eventErr != nil {
			return fmt.Errorf("%v; additionally failed to persist tool failure event: %w", mutationErr, eventErr)
		}
		return mutationErr
	}

	completed, err := controlplane.NewEvent(controlplane.NewEventRequest{
		RunID: options.runID,
		TaskID: options.taskID,
		Type: controlplane.EventToolCompleted,
		Summary: "github/create-pull-request completed",
		Data: map[string]any{
			"tool_id": "github",
			"operation": "create-pull-request",
			"request_digest": digest,
			"approval_id": approval.ID,
			"url": result.URL,
		},
	}, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("pull request created at %s but completion audit event could not be built: %w", result.URL, err)
	}
	if _, err := eventStore.Append(context.Background(), completed); err != nil {
		return fmt.Errorf("pull request created at %s but completion audit event could not be persisted: %w", result.URL, err)
	}

	return printGitHubPRView(githubPRView{
		RunID: options.runID, TaskID: options.taskID,
		Repository: options.repository, Head: options.head, Base: options.base,
		Title: options.title, RequestDigest: digest, Status: "created", URL: result.URL,
	}, options.json)
}


 // githubPRApprovalRecorded requires both a pending request and the exact
 // resolution event that references the stored immutable evidence. An orphan
 // evidence file (e.g. an audit append failure) cannot authorize a mutation.
func githubPRApprovalRecorded(events []controlplane.Event, taskID, digest, evidenceID string) bool {
	pending := false
	approved := false
	for _, event := range events {
		if event.TaskID != taskID {
			continue
		}
		var data struct {
			Kind string `json:"kind"`
			ToolID string `json:"tool_id"`
			Operation string `json:"operation"`
			RequestDigest string `json:"request_digest"`
			EvidenceID string `json:"evidence_id"`
		}
		if err := json.Unmarshal(event.Data, &data); err != nil ||
			data.Kind != "tool-operation" || data.ToolID != "github" ||
			data.Operation != "create-pull-request" || data.RequestDigest != digest {
			continue
		}
		switch event.Type {
		case controlplane.EventApprovalRequested:
			pending = true
			approved = false
		case controlplane.EventApprovalGranted:
			approved = pending && data.EvidenceID == evidenceID
			pending = false
		case controlplane.EventApprovalRejected:
			approved = false
			pending = false
		}
	}
	return approved
}

// githubPRMutationStarted treats a pre-mutation event as an irreversible
// attempt, even if a completion or failure event was never recorded. This is a
// sequential replay guard; concurrent CLI processes require a separate lock.
func githubPRMutationStarted(events []controlplane.Event, taskID, digest string) bool {
	for _, event := range events {
		if event.TaskID != taskID || event.Type != controlplane.EventToolStarted {
			continue
		}
		var data struct {
			ToolID string `json:"tool_id"`
			Operation string `json:"operation"`
			RequestDigest string `json:"request_digest"`
		}
		if json.Unmarshal(event.Data, &data) == nil &&
			data.ToolID == "github" && data.Operation == "create-pull-request" &&
			data.RequestDigest == digest {
			return true
		}
	}
	return false
}

func githubPRRequest(options githubPROptions, root string) githubintegration.CreatePullRequestRequest {
	return githubintegration.CreatePullRequestRequest{
		RunID: options.runID,
		TaskID: options.taskID,
		Root: root,
		Repository: strings.TrimSpace(options.repository),
		Head: strings.TrimSpace(options.head),
		Base: strings.TrimSpace(options.base),
		Title: strings.TrimSpace(options.title),
		Body: options.body,
	}
}

func findPullRequestApproval(
	approvals []toolapproval.Evidence,
	runID string,
	taskID string,
	digest string,
	now time.Time,
) (toolapproval.Evidence, bool) {
	for i := len(approvals) - 1; i >= 0; i-- {
		if toolapproval.GrantsRequest(
			approvals[i], runID, taskID, "github", "create-pull-request", digest, now,
		) {
			return approvals[i], true
		}
	}
	return toolapproval.Evidence{}, false
}

func parseGitHubPROptions(args []string, creating bool) (githubPROptions, error) {
	var options githubPROptions
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--task":
			if i+1 >= len(args) { return options, errors.New("--task requiere un id") }
			i++; options.taskID = args[i]
		case "--repository":
			if i+1 >= len(args) { return options, errors.New("--repository requiere owner/name") }
			i++; options.repository = args[i]
		case "--head":
			if i+1 >= len(args) { return options, errors.New("--head requiere una rama") }
			i++; options.head = args[i]
		case "--base":
			if i+1 >= len(args) { return options, errors.New("--base requiere una rama") }
			i++; options.base = args[i]
		case "--title":
			if i+1 >= len(args) { return options, errors.New("--title requiere texto") }
			i++; options.title = args[i]
		case "--body":
			if i+1 >= len(args) { return options, errors.New("--body requiere texto") }
			i++; options.body = args[i]
		case "--state-dir":
			if i+1 >= len(args) { return options, errors.New("--state-dir requiere una ruta") }
			i++; options.stateDir = args[i]
		case "--events-dir":
			if i+1 >= len(args) { return options, errors.New("--events-dir requiere una ruta") }
			i++; options.eventsDir = args[i]
		case "--approval-dir":
			if !creating {
				return options, errors.New("--approval-dir solo es válido al crear el pull request")
			}
			if i+1 >= len(args) { return options, errors.New("--approval-dir requiere una ruta") }
			i++; options.approvalDir = args[i]
		case "--json":
			options.json = true
		default:
			if strings.HasPrefix(args[i], "-") {
				return options, fmt.Errorf("opción desconocida: %s", args[i])
			}
			if options.runID != "" {
				return options, errors.New("se esperaba un solo run id")
			}
			options.runID = args[i]
		}
	}
	if strings.TrimSpace(options.runID) == "" ||
		strings.TrimSpace(options.taskID) == "" ||
		strings.TrimSpace(options.repository) == "" ||
		strings.TrimSpace(options.head) == "" ||
		strings.TrimSpace(options.base) == "" ||
		strings.TrimSpace(options.title) == "" {
		return options, errors.New("run id, --task, --repository, --head, --base y --title son obligatorios")
	}
	return options, nil
}

func printGitHubPRView(view githubPRView, asJSON bool) error {
	if asJSON {
		return printJSON(view)
	}
	fmt.Println("GitHub Pull Request")
	fmt.Printf("Run: %s\n", view.RunID)
	fmt.Printf("Task: %s\n", view.TaskID)
	fmt.Printf("Repository: %s\n", view.Repository)
	fmt.Printf("Head -> base: %s -> %s\n", view.Head, view.Base)
	fmt.Printf("Title: %s\n", view.Title)
	fmt.Printf("Request digest: %s\n", view.RequestDigest)
	fmt.Printf("Status: %s\n", view.Status)
	if view.URL != "" {
		fmt.Printf("URL: %s\n", view.URL)
	}
	return nil
}
