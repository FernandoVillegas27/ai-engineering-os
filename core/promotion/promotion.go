// Package promotion defines the explicit boundary between an approved staged
// workspace and the original project. P3a is dry-run only: it proves whether a
// staged result is safe to promote but never writes to the original project.
package promotion

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/staging"
)

type Kind string

const (
	KindPreImageMissing    Kind = "pre-image-missing"
	KindQualityNotPassed   Kind = "quality-not-passed"
	KindUnsafeConfig       Kind = "unsafe-configuration"
	KindOriginalChanged    Kind = "original-changed"
	KindStagedInvalid      Kind = "staged-invalid-workspace"
	KindUnauthorizedChange Kind = "unauthorized-change"
)

var ErrPromotion = errors.New("promotion validation failed")

type Error struct {
	Kind   Kind
	Detail string
	Paths  []string
}

func (e *Error) Error() string {
	var b strings.Builder
	b.WriteString("promotion validation failed (" + string(e.Kind) + ")")
	if strings.TrimSpace(e.Detail) != "" {
		b.WriteString(": " + e.Detail)
	}
	if len(e.Paths) > 0 {
		paths := append([]string(nil), e.Paths...)
		sort.Strings(paths)
		b.WriteString("; paths: " + strings.Join(paths, ", "))
	}
	return b.String()
}

func (e *Error) Is(target error) bool { return target == ErrPromotion }

type Request struct {
	OriginalRoot    string
	StagedWorkspace string
	PreImage        staging.PreImage
	Declared        []staging.WorkspaceChange
	QualityPassed   bool
	TempParent      string
}

type Plan struct {
	Ready          bool
	PreImageDigest string
	Changes        []staging.WorkspaceChange
}

func DryRun(ctx context.Context, request Request) (Plan, error) {
	if err := ctx.Err(); err != nil {
		return Plan{}, fmt.Errorf("promotion canceled: %w", err)
	}
	if request.PreImage.IsZero() {
		return Plan{}, &Error{Kind: KindPreImageMissing, Detail: "the stage result carries no pre-image"}
	}
	if !request.QualityPassed {
		return Plan{}, &Error{Kind: KindQualityNotPassed, Detail: "quality gate has not passed"}
	}
	if len(request.Declared) == 0 {
		return Plan{}, &Error{Kind: KindUnsafeConfig, Detail: "declared promotion scope is empty"}
	}

	root, err := filepath.Abs(request.OriginalRoot)
	if err != nil {
		return Plan{}, fmt.Errorf("resolve original project: %w", err)
	}
	staged, err := filepath.Abs(request.StagedWorkspace)
	if err != nil {
		return Plan{}, fmt.Errorf("resolve staged workspace: %w", err)
	}
	rootInfo, err := os.Stat(root)
	if err != nil || !rootInfo.IsDir() {
		return Plan{}, &Error{Kind: KindUnsafeConfig, Detail: "original project must be an existing directory"}
	}
	stagedInfo, err := os.Stat(staged)
	if err != nil || !stagedInfo.IsDir() {
		return Plan{}, &Error{Kind: KindStagedInvalid, Detail: "staged workspace must be an existing directory"}
	}
	if os.SameFile(rootInfo, stagedInfo) {
		return Plan{}, &Error{Kind: KindUnsafeConfig, Detail: "staged workspace must be separate from the original project"}
	}

	policy := execution.DefaultSafeViewPolicy()
	declared := make([]staging.WorkspaceChange, 0, len(request.Declared))
	declaredByPath := map[string]string{}
	for _, change := range request.Declared {
		clean, err := staging.ValidateWorkspacePath(change.Path, policy)
		if err != nil {
			return Plan{}, &Error{Kind: KindUnsafeConfig, Detail: fmt.Sprintf("invalid declared path %q: %v", change.Path, err)}
		}
		action := strings.ToLower(strings.TrimSpace(change.Action))
		if action != "create" && action != "modify" && action != "delete" {
			return Plan{}, &Error{Kind: KindUnsafeConfig, Detail: fmt.Sprintf("invalid declared action %q for %s", change.Action, clean)}
		}
		if _, exists := declaredByPath[clean]; exists {
			return Plan{}, &Error{Kind: KindUnsafeConfig, Detail: "duplicate declared path", Paths: []string{clean}}
		}
		declaredByPath[clean] = action
		declared = append(declared, staging.WorkspaceChange{Path: clean, Action: action})
	}

	if err := verifyOriginal(ctx, root, request.PreImage, request.TempParent, policy); err != nil {
		return Plan{}, err
	}

	changes, err := staging.DetectWorkspaceChanges(request.PreImage.Snapshot(), staged, policy)
	if err != nil {
		return Plan{}, &Error{Kind: KindStagedInvalid, Detail: scrubPath(err.Error(), staged)}
	}
	if len(changes) == 0 {
		return Plan{}, &Error{Kind: KindStagedInvalid, Detail: "staged workspace contains no promotable changes"}
	}

	var unauthorized []string
	for _, change := range changes {
		action, ok := declaredByPath[change.Path]
		if !ok || action != change.Action {
			unauthorized = append(unauthorized, change.Path)
		}
	}
	if len(unauthorized) > 0 {
		return Plan{}, &Error{
			Kind:   KindUnauthorizedChange,
			Detail: "staged changes do not match the declared promotion scope",
			Paths:  unauthorized,
		}
	}

	sort.Slice(changes, func(i, j int) bool { return changes[i].Path < changes[j].Path })
	return Plan{
		Ready:          true,
		PreImageDigest: request.PreImage.Digest(),
		Changes:        append([]staging.WorkspaceChange(nil), changes...),
	}, nil
}

func verifyOriginal(ctx context.Context, root string, preImage staging.PreImage, tempParent string, policy execution.SafeViewPolicy) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("promotion canceled: %w", err)
	}
	parent := strings.TrimSpace(tempParent)
	if parent == "" {
		parent = filepath.Dir(root)
	}
	parent, err := filepath.Abs(parent)
	if err != nil {
		return fmt.Errorf("resolve promotion temporary parent: %w", err)
	}
	info, err := os.Stat(parent)
	if err != nil || !info.IsDir() {
		return &Error{Kind: KindUnsafeConfig, Detail: "promotion temporary parent must be an existing directory"}
	}
	if within(root, parent) {
		return &Error{Kind: KindUnsafeConfig, Detail: "promotion temporary parent must be outside the original project"}
	}

	view, err := os.MkdirTemp(parent, ".ai-engineering-os-promotion-*")
	if err != nil {
		return fmt.Errorf("create promotion validation workspace: %w", err)
	}
	defer os.RemoveAll(view)

	if _, err := execution.PrepareSafeReadOnlyViewWithPolicy(root, view, policy); err != nil {
		return &Error{Kind: KindOriginalChanged, Detail: scrubPath(err.Error(), root, view)}
	}
	snapshot, err := staging.SnapshotWorkspace(view, policy)
	if err != nil {
		return &Error{Kind: KindOriginalChanged, Detail: scrubPath(err.Error(), root, view)}
	}
	if diff := preImage.DiffPaths(snapshot); len(diff) > 0 {
		return &Error{
			Kind:   KindOriginalChanged,
			Detail: "original project no longer matches the state that was staged",
			Paths:  diff,
		}
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("promotion canceled: %w", err)
	}
	return nil
}

func within(root, candidate string) bool {
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func scrubPath(message string, locations ...string) string {
	for _, location := range locations {
		if strings.TrimSpace(location) != "" {
			message = strings.ReplaceAll(message, location, "<path>")
		}
	}
	return message
}
