// Package baseline verifies the untouched original project so a later comparison
// can tell pre-existing failures from new ones. It never writes to the original
// project and refuses to produce a baseline whose integrity it cannot prove.
package baseline

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/staging"
	"github.com/FernandoVillegas27/ai-engineering-os/core/verification"
)

const (
	tempPattern    = ".ai-engineering-os-baseline-*"
	maxListedPaths = 10
)

// ErrIntegrity matches every IntegrityError with errors.Is.
var ErrIntegrity = errors.New("baseline integrity violation")

// Kind names the invariant that failed.
type Kind string

const (
	KindPreImageMissing    Kind = "pre-image-missing"
	KindUnsafeConfig       Kind = "unsafe-configuration"
	KindOriginalChanged    Kind = "original-changed"
	KindStagedUnauthorized Kind = "staged-unauthorized-change"
	KindStagedInvalid      Kind = "staged-invalid-workspace"
)

// IntegrityError is a blocking failure. Callers must not continue to correction
// or any model call when they receive one. It carries relative paths and counts only.
type IntegrityError struct {
	Kind   Kind
	Detail string
	Paths  []string
}

func (e *IntegrityError) Error() string {
	var message strings.Builder
	message.WriteString("baseline integrity violation (" + string(e.Kind) + ")")
	if e.Detail != "" {
		message.WriteString(": " + e.Detail)
	}
	if len(e.Paths) > 0 {
		listed := e.Paths
		if len(listed) > maxListedPaths {
			listed = listed[:maxListedPaths]
		}
		message.WriteString("; paths: " + strings.Join(listed, ", "))
		if extra := len(e.Paths) - len(listed); extra > 0 {
			message.WriteString(fmt.Sprintf(" (+%d more)", extra))
		}
	}
	return message.String()
}

func (e *IntegrityError) Is(target error) bool { return target == ErrIntegrity }

func integrity(kind Kind, detail string, paths []string) *IntegrityError {
	sorted := append([]string(nil), paths...)
	sort.Strings(sorted)
	return &IntegrityError{Kind: kind, Detail: detail, Paths: sorted}
}

// Config binds a provider to one staging run.
type Config struct {
	// OriginalRoot is the project that was staged. It is only ever read.
	OriginalRoot string
	// PreImage comes from the stage result and anchors every comparison.
	PreImage staging.PreImage
	// Authorized lists the workspace-relative files the agent may change.
	Authorized []string
	// Verifier must be the same verifier used on the staged workspace.
	Verifier verification.Runtime
	// TempParent defaults to the parent of OriginalRoot and must lie outside it.
	TempParent string
}

// Result is a verified baseline.
type Result struct {
	Report         verification.Report
	PreImageDigest string
	// StagedChanges lists the paths in which the staged workspace currently differs from the pre-image.
	StagedChanges []string
	Warnings      []string
}

// Provider produces the baseline lazily and at most once per valid state.
type Provider struct {
	root       string
	tempParent string
	preImage   staging.PreImage
	authorized map[string]bool
	verifier   verification.Runtime
	policy     execution.SafeViewPolicy

	mu     sync.Mutex
	cached *Result
}

// New validates the configuration. It uses the same default safe-view policy as the
// stage runtimes, so the baseline view is filtered exactly like the staged one.
func New(config Config) (*Provider, error) {
	if config.Verifier == nil {
		return nil, errors.New("baseline verifier is required")
	}
	if config.PreImage.IsZero() {
		return nil, integrity(KindPreImageMissing, "the stage result carries no pre-image", nil)
	}
	root, err := filepath.Abs(config.OriginalRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve original project: %w", err)
	}
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		return nil, errors.New("original project must be an existing directory")
	}
	parent := config.TempParent
	if strings.TrimSpace(parent) == "" {
		parent = filepath.Dir(root)
	}
	parent, err = filepath.Abs(parent)
	if err != nil {
		return nil, fmt.Errorf("resolve temporary parent: %w", err)
	}
	if info, err := os.Stat(parent); err != nil || !info.IsDir() {
		return nil, errors.New("temporary parent must be an existing directory")
	}
	if within(root, parent) {
		return nil, integrity(KindUnsafeConfig, "the temporary parent must be outside the original project", nil)
	}

	policy := execution.DefaultSafeViewPolicy()
	authorized := map[string]bool{}
	for _, value := range config.Authorized {
		clean, err := staging.ValidateWorkspacePath(value, policy)
		if err != nil {
			return nil, fmt.Errorf("authorized file %q: %w", value, err)
		}
		authorized[clean] = true
	}

	return &Provider{
		root:       root,
		tempParent: parent,
		preImage:   config.PreImage,
		authorized: authorized,
		verifier:   config.Verifier,
		policy:     policy,
	}, nil
}

// Baseline returns the verification of the untouched original. On every call it
// first re-checks the staged workspace, and before reusing a memoized result it
// re-checks the original. Any violation yields a blocking error and no result.
func (p *Provider) Baseline(ctx context.Context, stagedWorkspace string) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, fmt.Errorf("baseline canceled: %w", err)
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	changes, err := p.checkStaged(stagedWorkspace)
	if err != nil {
		return Result{}, err
	}

	if p.cached != nil {
		warnings, err := p.checkOriginal(ctx)
		if err != nil {
			p.cached = nil
			return Result{}, err
		}
		result := cloneResult(*p.cached)
		result.StagedChanges = changes
		result.Warnings = warnings
		return result, nil
	}

	report, warnings, err := p.runBaseline(ctx)
	if err != nil {
		return Result{}, err
	}
	// The original must have stayed identical for the whole verification window.
	postWarnings, err := p.checkOriginal(ctx)
	if err != nil {
		return Result{}, err
	}

	result := Result{
		Report:         report,
		PreImageDigest: p.preImage.Digest(),
		StagedChanges:  changes,
		Warnings:       append(warnings, postWarnings...),
	}
	memo := cloneResult(result)
	memo.StagedChanges, memo.Warnings = nil, nil
	p.cached = &memo
	return result, nil
}

func (p *Provider) runBaseline(ctx context.Context) (report verification.Report, warnings []string, err error) {
	dir, cleanup, viewErr := p.newView(ctx)
	if cleanup != nil {
		defer func() {
			if warning := cleanup(); warning != "" {
				warnings = append(warnings, warning)
			}
		}()
	}
	if viewErr != nil {
		return verification.Report{}, nil, viewErr
	}

	verified, verifyErr := p.verifier.Verify(ctx, dir)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return verification.Report{}, nil, fmt.Errorf("baseline verification canceled: %w", ctxErr)
	}
	if verifyErr != nil {
		return verification.Report{}, nil, fmt.Errorf("baseline verification could not run: %w", verifyErr)
	}
	return cloneReport(verified), nil, nil
}

// checkOriginal builds a throwaway view of the original and compares it with the pre-image.
func (p *Provider) checkOriginal(ctx context.Context) ([]string, error) {
	_, cleanup, err := p.newView(ctx)
	var warnings []string
	if cleanup != nil {
		if warning := cleanup(); warning != "" {
			warnings = append(warnings, warning)
		}
	}
	return warnings, err
}

// newView copies the original through the safe-view policy and proves it equals the pre-image.
func (p *Provider) newView(ctx context.Context) (string, func() string, error) {
	if err := ctx.Err(); err != nil {
		return "", nil, fmt.Errorf("baseline canceled: %w", err)
	}
	dir, err := os.MkdirTemp(p.tempParent, tempPattern)
	if err != nil {
		return "", nil, fmt.Errorf("create baseline workspace: %w", err)
	}
	cleanup := func() string {
		if err := os.RemoveAll(dir); err != nil {
			return "temporary baseline directory " + filepath.Base(dir) + " could not be removed"
		}
		return ""
	}

	if _, err := execution.PrepareSafeReadOnlyView(p.root, dir); err != nil {
		return "", cleanup, fmt.Errorf("prepare baseline view: %s", p.scrub(err.Error(), dir))
	}
	snapshot, err := staging.SnapshotWorkspace(dir, p.policy)
	if err != nil {
		return "", cleanup, integrity(KindOriginalChanged, "the baseline view could not be inspected: "+p.scrub(err.Error(), dir), nil)
	}
	if diff := p.preImage.DiffPaths(snapshot); len(diff) > 0 {
		return "", cleanup, integrity(KindOriginalChanged, "the original project no longer matches the state that was staged", diff)
	}
	if err := ctx.Err(); err != nil {
		return "", cleanup, fmt.Errorf("baseline canceled: %w", err)
	}
	return dir, cleanup, nil
}

// checkStaged proves the staged workspace differs from the pre-image only in authorized files.
func (p *Provider) checkStaged(workspace string) ([]string, error) {
	abs, err := filepath.Abs(workspace)
	if err != nil {
		return nil, integrity(KindStagedInvalid, "cannot resolve the staged workspace", nil)
	}
	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		return nil, integrity(KindStagedInvalid, "the staged workspace is not an existing directory", nil)
	}
	if rootInfo, err := os.Stat(p.root); err == nil && os.SameFile(rootInfo, info) {
		return nil, integrity(KindUnsafeConfig, "the staged workspace is the original project", nil)
	}

	changes, err := staging.DetectWorkspaceChanges(p.preImage.Snapshot(), abs, p.policy)
	if err != nil {
		return nil, integrity(KindStagedInvalid, p.scrub(err.Error(), abs), nil)
	}
	changed := make([]string, 0, len(changes))
	var unauthorized []string
	for _, change := range changes {
		changed = append(changed, change.Path)
		if !p.authorized[change.Path] {
			unauthorized = append(unauthorized, change.Path)
		}
	}
	if len(unauthorized) > 0 {
		return nil, integrity(KindStagedUnauthorized, "the staged workspace changed files that were not authorized", unauthorized)
	}
	sort.Strings(changed)
	return changed, nil
}

// scrub removes absolute workspace locations from messages.
func (p *Provider) scrub(message string, extra ...string) string {
	for _, location := range append([]string{p.root, p.tempParent}, extra...) {
		if location != "" {
			message = strings.ReplaceAll(message, location, "<path>")
		}
	}
	return message
}

// within reports whether candidate is root itself or lies inside it.
func within(root, candidate string) bool {
	resolve := func(path string) string {
		if resolved, err := filepath.EvalSymlinks(path); err == nil {
			return resolved
		}
		return path
	}
	relative, err := filepath.Rel(resolve(root), resolve(candidate))
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative))
}

func cloneResult(result Result) Result {
	result.Report = cloneReport(result.Report)
	result.StagedChanges = append([]string(nil), result.StagedChanges...)
	result.Warnings = append([]string(nil), result.Warnings...)
	return result
}

func cloneReport(report verification.Report) verification.Report {
	clone := report
	clone.Checks = make([]verification.Check, len(report.Checks))
	for i, check := range report.Checks {
		check.Command = append([]string(nil), check.Command...)
		check.Failures = append([]verification.Failure(nil), check.Failures...)
		clone.Checks[i] = check
	}
	return clone
}
