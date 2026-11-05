package promotion

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/FernandoVillegas27/ai-engineering-os/core/staging"
)

func applyFixture(t *testing.T) (base, original, staged string, preImage staging.PreImage) {
	t.Helper()
	base = t.TempDir()
	original = filepath.Join(base, "project")
	staged = filepath.Join(base, "stage")
	if err := os.MkdirAll(original, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(staged, 0o755); err != nil {
		t.Fatal(err)
	}

	writeFile(t, original, "main.go", "package main\nconst value = 1\n")
	writeFile(t, original, "delete.go", "package main\nconst obsolete = true\n")
	writeFile(t, original, ".git/config", "git-metadata-must-remain")

	preImage = preImageFromOriginal(t, original, base)

	writeFile(t, staged, "main.go", "package main\nconst value = 2\n")
	writeFile(t, staged, "create.go", "package main\nconst created = true\n")
	return
}

func TestApplyRequiresExplicitApprovalWithoutMutation(t *testing.T) {
	_, original, staged, preImage := applyFixture(t)

	result, err := Apply(context.Background(), ApplyRequest{
		Request: Request{
			OriginalRoot:    original,
			StagedWorkspace: staged,
			PreImage:        preImage,
			Declared: []staging.WorkspaceChange{
				{Path: "main.go", Action: "modify"},
				{Path: "create.go", Action: "create"},
				{Path: "delete.go", Action: "delete"},
			},
			QualityPassed: true,
		},
		Approved: false,
	})
	var promotionErr *Error
	if !errors.As(err, &promotionErr) || promotionErr.Kind != KindApprovalRequired {
		t.Fatalf("err = %#v", err)
	}
	if result.Applied {
		t.Fatalf("result = %#v", result)
	}
	data, readErr := os.ReadFile(filepath.Join(original, "main.go"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(data) != "package main\nconst value = 1\n" {
		t.Fatalf("original changed without approval: %q", data)
	}
	if _, statErr := os.Stat(filepath.Join(original, "create.go")); !os.IsNotExist(statErr) {
		t.Fatalf("create.go should not exist, err=%v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(original, "delete.go")); statErr != nil {
		t.Fatalf("delete.go should still exist: %v", statErr)
	}
}

func TestApplyPromotesValidatedChangesAndPreservesExcludedOriginalData(t *testing.T) {
	base, original, staged, preImage := applyFixture(t)

	result, err := Apply(context.Background(), ApplyRequest{
		Request: Request{
			OriginalRoot:    original,
			StagedWorkspace: staged,
			PreImage:        preImage,
			Declared: []staging.WorkspaceChange{
				{Path: "main.go", Action: "modify"},
				{Path: "create.go", Action: "create"},
				{Path: "delete.go", Action: "delete"},
			},
			QualityPassed: true,
		},
		Approved: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Applied || result.RolledBack || len(result.Changes) != 3 {
		t.Fatalf("result = %#v", result)
	}

	mainData, err := os.ReadFile(filepath.Join(original, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(mainData) != "package main\nconst value = 2\n" {
		t.Fatalf("main.go = %q", mainData)
	}
	createData, err := os.ReadFile(filepath.Join(original, "create.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(createData) != "package main\nconst created = true\n" {
		t.Fatalf("create.go = %q", createData)
	}
	if _, err := os.Stat(filepath.Join(original, "delete.go")); !os.IsNotExist(err) {
		t.Fatalf("delete.go should be removed, err=%v", err)
	}
	gitData, err := os.ReadFile(filepath.Join(original, ".git", "config"))
	if err != nil {
		t.Fatal(err)
	}
	if string(gitData) != "git-metadata-must-remain" {
		t.Fatalf(".git/config changed: %q", gitData)
	}

	rollbackDirs, err := filepath.Glob(filepath.Join(base, ".ai-engineering-os-rollback-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(rollbackDirs) != 0 {
		t.Fatalf("rollback storage leaked after success: %#v", rollbackDirs)
	}
}

func TestApplyRevalidatesOriginalBeforeAnyWrite(t *testing.T) {
	_, original, staged, preImage := applyFixture(t)
	writeFile(t, original, "main.go", "package main\nconst value = 999\n")

	result, err := Apply(context.Background(), ApplyRequest{
		Request: Request{
			OriginalRoot:    original,
			StagedWorkspace: staged,
			PreImage:        preImage,
			Declared: []staging.WorkspaceChange{
				{Path: "main.go", Action: "modify"},
				{Path: "create.go", Action: "create"},
				{Path: "delete.go", Action: "delete"},
			},
			QualityPassed: true,
		},
		Approved: true,
	})
	var promotionErr *Error
	if !errors.As(err, &promotionErr) || promotionErr.Kind != KindOriginalChanged {
		t.Fatalf("err = %#v", err)
	}
	if result.Applied {
		t.Fatalf("result = %#v", result)
	}
	data, readErr := os.ReadFile(filepath.Join(original, "main.go"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(data) != "package main\nconst value = 999\n" {
		t.Fatalf("original unexpectedly changed: %q", data)
	}
}

func TestApplyDoesNotCreateNewDirectoryTrees(t *testing.T) {
	base := t.TempDir()
	original := filepath.Join(base, "project")
	staged := filepath.Join(base, "stage")
	if err := os.MkdirAll(original, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(staged, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, original, "main.go", "package main\n")
	preImage := preImageFromOriginal(t, original, base)
	writeFile(t, staged, "main.go", "package main\n")
	writeFile(t, staged, "newdir/new.go", "package newdir\n")

	result, err := Apply(context.Background(), ApplyRequest{
		Request: Request{
			OriginalRoot:    original,
			StagedWorkspace: staged,
			PreImage:        preImage,
			Declared: []staging.WorkspaceChange{
				{Path: "newdir/new.go", Action: "create"},
			},
			QualityPassed: true,
		},
		Approved: true,
	})
	var promotionErr *Error
	if !errors.As(err, &promotionErr) || promotionErr.Kind != KindApplyFailed {
		t.Fatalf("err = %#v", err)
	}
	if result.Applied {
		t.Fatalf("result = %#v", result)
	}
	if _, statErr := os.Stat(filepath.Join(original, "newdir")); !os.IsNotExist(statErr) {
		t.Fatalf("newdir should not be created, err=%v", statErr)
	}
}

func TestRollbackRestoresAppliedCreateModifyAndDelete(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "project")
	backup := filepath.Join(base, "backup")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(backup, 0o755); err != nil {
		t.Fatal(err)
	}

	writeFile(t, root, "main.go", "new-main")
	writeFile(t, root, "created.go", "new-created")
	writeFile(t, backup, "main.go", "old-main")
	writeFile(t, backup, "deleted.go", "old-deleted")

	applied := []staging.WorkspaceChange{
		{Path: "main.go", Action: "modify"},
		{Path: "created.go", Action: "create"},
		{Path: "deleted.go", Action: "delete"},
	}
	if err := rollbackApplied(root, backup, applied); err != nil {
		t.Fatal(err)
	}

	mainData, err := os.ReadFile(filepath.Join(root, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(mainData) != "old-main" {
		t.Fatalf("main.go = %q", mainData)
	}
	if _, err := os.Stat(filepath.Join(root, "created.go")); !os.IsNotExist(err) {
		t.Fatalf("created.go should be removed, err=%v", err)
	}
	deletedData, err := os.ReadFile(filepath.Join(root, "deleted.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(deletedData) != "old-deleted" {
		t.Fatalf("deleted.go = %q", deletedData)
	}
}
