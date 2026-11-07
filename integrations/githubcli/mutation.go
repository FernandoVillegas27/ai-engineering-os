package githubcli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/toolapproval"
	"github.com/FernandoVillegas27/ai-engineering-os/core/toolhealth"
)

var repositoryNamePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

type CreatePullRequestRequest struct {
	RunID      string `json:"run_id"`
	TaskID     string `json:"task_id"`
	Root       string `json:"-"`
	Repository string `json:"repository"`
	Head       string `json:"head"`
	Base       string `json:"base"`
	Title      string `json:"title"`
	Body       string `json:"body"`
}

type CreatePullRequestResult struct {
	URL           string
	RequestDigest string
	Duration      time.Duration
}

func (request CreatePullRequestRequest) RequestDigest() (string, error) {
	payload, err := canonicalPullRequestPayload(request)
	if err != nil {
		return "", err
	}
	return toolapproval.DigestRequest(payload), nil
}

func (c Client) CreatePullRequest(
	ctx context.Context,
	request CreatePullRequestRequest,
	approval toolapproval.Evidence,
) (CreatePullRequestResult, error) {
	absolute, err := validateCreatePullRequest(request)
	if err != nil {
		return CreatePullRequestResult{}, err
	}
	digest, err := request.RequestDigest()
	if err != nil {
		return CreatePullRequestResult{}, err
	}
	now := time.Now().UTC()
	if !toolapproval.GrantsRequest(
		approval,
		request.RunID,
		request.TaskID,
		"github",
		"create-pull-request",
		digest,
		now,
	) {
		return CreatePullRequestResult{}, errors.New("GitHub create-pull-request requires matching non-expired human approval evidence")
	}

	binary := strings.TrimSpace(c.Binary)
	if binary == "" {
		binary = "gh"
	}
	runner := c.Runner
	if runner == nil {
		runner = OSRunner{}
	}
	path, err := runner.LookPath(binary)
	if err != nil {
		return CreatePullRequestResult{}, fmt.Errorf("GitHub CLI unavailable: %w", err)
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	env := githubEnvironment()

	_, failure, duration := toolhealth.Run(ctx, timeout, func(runCtx context.Context) (string, error) {
		return runner.Run(runCtx, Invocation{
			Dir: absolute,
			Env: env,
			Name: path,
			Args: []string{"auth", "status"},
		})
	})
	if failure.Kind != toolhealth.FailureNone {
		return CreatePullRequestResult{}, fmt.Errorf(
			"GitHub CLI authentication [%s]: %s",
			failure.Kind,
			toolhealth.Redact(failure.Detail),
		)
	}

	repoOutput, repoFailure, repoDuration := toolhealth.Run(ctx, timeout, func(runCtx context.Context) (string, error) {
		return runner.Run(runCtx, Invocation{
			Dir: absolute,
			Env: env,
			Name: path,
			Args: []string{"repo", "view", "--json", "nameWithOwner"},
		})
	})
	duration += repoDuration
	if repoFailure.Kind != toolhealth.FailureNone {
		return CreatePullRequestResult{}, fmt.Errorf(
			"verify GitHub repository [%s]: %s",
			repoFailure.Kind,
			toolhealth.Redact(repoFailure.Detail),
		)
	}
	var repoIdentity struct {
		NameWithOwner string `json:"nameWithOwner"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(repoOutput)), &repoIdentity); err != nil {
		return CreatePullRequestResult{}, errors.New("GitHub repository identity response is invalid JSON")
	}
	if strings.TrimSpace(repoIdentity.NameWithOwner) != request.Repository {
		return CreatePullRequestResult{}, fmt.Errorf(
			"GitHub repository identity mismatch: got %q want %q",
			strings.TrimSpace(repoIdentity.NameWithOwner),
			request.Repository,
		)
	}

	output, mutationFailure, mutationDuration := toolhealth.Run(ctx, timeout, func(runCtx context.Context) (string, error) {
		return runner.Run(runCtx, Invocation{
			Dir: absolute,
			Env: env,
			Name: path,
			Args: []string{
				"pr", "create",
				"--repo", request.Repository,
				"--head", request.Head,
				"--base", request.Base,
				"--title", request.Title,
				"--body", request.Body,
			},
		})
	})
	duration += mutationDuration
	if mutationFailure.Kind != toolhealth.FailureNone {
		return CreatePullRequestResult{}, fmt.Errorf(
			"GitHub create-pull-request [%s]: %s",
			mutationFailure.Kind,
			toolhealth.Redact(mutationFailure.Detail),
		)
	}
	prURL := strings.TrimSpace(output)
	parsed, err := url.Parse(prURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || !strings.Contains(parsed.Path, "/pull/") {
		return CreatePullRequestResult{}, errors.New("GitHub create-pull-request returned an invalid pull-request URL")
	}
	return CreatePullRequestResult{
		URL: prURL,
		RequestDigest: digest,
		Duration: duration,
	}, nil
}

func canonicalPullRequestPayload(request CreatePullRequestRequest) ([]byte, error) {
	request.Repository = strings.TrimSpace(request.Repository)
	request.Head = strings.TrimSpace(request.Head)
	request.Base = strings.TrimSpace(request.Base)
	request.Title = strings.TrimSpace(request.Title)
	if request.Repository == "" || request.Head == "" || request.Base == "" || request.Title == "" {
		return nil, errors.New("pull-request repository, head, base and title are required")
	}
	return json.Marshal(struct {
		Repository string `json:"repository"`
		Head       string `json:"head"`
		Base       string `json:"base"`
		Title      string `json:"title"`
		Body       string `json:"body"`
	}{
		Repository: request.Repository,
		Head: request.Head,
		Base: request.Base,
		Title: request.Title,
		Body: request.Body,
	})
}

func validateCreatePullRequest(request CreatePullRequestRequest) (string, error) {
	if strings.TrimSpace(request.RunID) == "" || strings.TrimSpace(request.TaskID) == "" {
		return "", errors.New("GitHub create-pull-request requires run and task ids")
	}
	root := strings.TrimSpace(request.Root)
	if root == "" {
		return "", errors.New("GitHub create-pull-request requires project root")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", errors.New("GitHub create-pull-request project root must be a directory")
	}
	repository := strings.TrimSpace(request.Repository)
	if !repositoryNamePattern.MatchString(repository) || strings.Contains(repository, "..") {
		return "", errors.New("GitHub repository must be an explicit owner/name")
	}
	if err := validateRef("head", request.Head); err != nil {
		return "", err
	}
	if err := validateRef("base", request.Base); err != nil {
		return "", err
	}
	title := strings.TrimSpace(request.Title)
	if title == "" || len(title) > 256 || strings.ContainsAny(title, "\r\n") {
		return "", errors.New("pull-request title must be one line and at most 256 bytes")
	}
	if len(request.Body) > 16*1024 {
		return "", errors.New("pull-request body exceeds 16 KiB")
	}
	return absolute, nil
}

func validateRef(label, value string) error {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 255 {
		return fmt.Errorf("pull-request %s ref is invalid", label)
	}
	if strings.HasPrefix(value, "-") || strings.Contains(value, "..") ||
		strings.Contains(value, "@{") || strings.ContainsAny(value, " ~^:?*[\\") ||
		strings.HasSuffix(value, "/") || strings.HasSuffix(value, ".") {
		return fmt.Errorf("pull-request %s ref is invalid", label)
	}
	return nil
}
