package promotion

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/staging"
)

func writeFile(t *testing.T, root, relative, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func preImageFromOriginal(t *testing.T, root, parent string) staging.PreImage {
	t.Helper()
	view := filepath.Join(parent, "preimage-view")
	if _, err := execution.PrepareSafeReadOnlyView(root, view); err != nil {
		t.Fatal(err)
	}
	snapshot, err := staging.SnapshotWorkspace(view, execution.DefaultSafeViewPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(view); err != nil {
		t.Fatal(err)
	}
	return staging.NewPreImage(snapshot)
}

func promotionFixture(t *testing.T) (base, original, staged string, preImage staging.PreImage) {
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
	writeFile(t, original, ".git/config", "must stay outside safe view")
	preImage = preImageFromOriginal(t, original, base)
	writeFile(t, staged, "main.go", "package main\nconst value = 1\n")
	return
}

func TestDryRunProducesReadyPlanWithoutMutatingOriginal(t *testing.T) {
	base, original, staged, preImage := promotionFixture(t)
	writeFile(t, staged, "main.go", "package main\nconst value = 2\n")

	plan, err := DryRun(context.Background(), Request{
		OriginalRoot:    original,
		StagedWorkspace: staged,
		PreImage:        preImage,
		Declared:        []staging.WorkspaceChange{{Path: "main.go", Action: "modify"}},
		QualityPassed:   true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Ready || plan.PreImageDigest != preImage.Digest() {
		t.Fatalf("plan = %#v", plan)
	}
	if len(plan.Changes) != 1 || plan.Changes[0].Path != "main.go" || plan.Changes[0].Action != "modify" {
		t.Fatalf("changes = %#v", plan.Changes)
	}
	data, err := os.ReadFile(filepath.Join(original, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "package main\nconst value = 1\n" {
		t.Fatalf("original was mutated: %q", data)
	}
	tempDirs, err := filepath.Glob(filepath.Join(base, ".ai-engineering-os-promotion-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(tempDirs) != 0 {
		t.Fatalf("promotion temp directories leaked: %#v", tempDirs)
	}
}

func TestDryRunRejectsChangedOriginal(t *testing.T) {
	_, original, staged, preImage := promotionFixture(t)
	writeFile(t, original, "main.go", "package main\nconst value = 99\n")
	writeFile(t, staged, "main.go", "package main\nconst value = 2\n")

	_, err := DryRun(context.Background(), Request{
		OriginalRoot:    original,
		StagedWorkspace: staged,
		PreImage:        preImage,
		Declared:        []staging.WorkspaceChange{{Path: "main.go", Action: "modify"}},
		QualityPassed:   true,
	})
	var promotionErr *Error
	if !errors.As(err, &promotionErr) || promotionErr.Kind != KindOriginalChanged {
		t.Fatalf("err = %#v", err)
	}
}

func TestDryRunRejectsUnauthorizedStagedChange(t *testing.T) {
	_, original, staged, preImage := promotionFixture(t)
	writeFile(t, staged, "main.go", "package main\nconst value = 2\n")
	writeFile(t, staged, "extra.go", "package main\n")

	_, err := DryRun(context.Background(), Request{
		OriginalRoot:    original,
		StagedWorkspace: staged,
		PreImage:        preImage,
		Declared:        []staging.WorkspaceChange{{Path: "main.go", Action: "modify"}},
		QualityPassed:   true,
	})
	var promotionErr *Error
	if !errors.As(err, &promotionErr) || promotionErr.Kind != KindUnauthorizedChange {
		t.Fatalf("err = %#v", err)
	}
	if len(promotionErr.Paths) != 1 || promotionErr.Paths[0] != "extra.go" {
		t.Fatalf("paths = %#v", promotionErr.Paths)
	}
}

func TestDryRunRejectsDeclaredActionMismatch(t *testing.T) {
	_, original, staged, preImage := promotionFixture(t)
	writeFile(t, staged, "main.go", "package main\nconst value = 2\n")

	_, err := DryRun(context.Background(), Request{
		OriginalRoot:    original,
		StagedWorkspace: staged,
		PreImage:        preImage,
		Declared:        []staging.WorkspaceChange{{Path: "main.go", Action: "create"}},
		QualityPassed:   true,
	})
	var promotionErr *Error
	if !errors.As(err, &promotionErr) || promotionErr.Kind != KindUnauthorizedChange {
		t.Fatalf("err = %#v", err)
	}
}

func TestDryRunRequiresPassedQualityGate(t *testing.T) {
	_, err := DryRun(context.Background(), Request{QualityPassed: false})
	var promotionErr *Error
	if !errors.As(err, &promotionErr) || promotionErr.Kind != KindPreImageMissing {
		t.Fatalf("zero pre-image must fail first, err = %#v", err)
	}

	preImage := staging.NewPreImage(staging.WorkspaceSnapshot{})
	_, err = DryRun(context.Background(), Request{PreImage: preImage, QualityPassed: false})
	if !errors.As(err, &promotionErr) || promotionErr.Kind != KindQualityNotPassed {
		t.Fatalf("err = %#v", err)
	}
}

func TestDryRunRejectsUnsafeStagedWorkspace(t *testing.T) {
	_, original, staged, preImage := promotionFixture(t)
	writeFile(t, staged, "main.go", "package main\nconst value = 2\n")
	writeFile(t, staged, ".env", "SECRET=value")

	_, err := DryRun(context.Background(), Request{
		OriginalRoot:    original,
		StagedWorkspace: staged,
		PreImage:        preImage,
		Declared:        []staging.WorkspaceChange{{Path: "main.go", Action: "modify"}},
		QualityPassed:   true,
	})
	var promotionErr *Error
	if !errors.As(err, &promotionErr) || promotionErr.Kind != KindStagedInvalid {
		t.Fatalf("err = %#v", err)
	}
}

func TestDryRunRejectsOriginalAsStagedWorkspace(t *testing.T) {
	base := t.TempDir()
	original := filepath.Join(base, "project")
	if err := os.MkdirAll(original, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, original, "main.go", "package main\n")
	preImage := preImageFromOriginal(t, original, base)

	_, err := DryRun(context.Background(), Request{
		OriginalRoot:    original,
		StagedWorkspace: original,
		PreImage:        preImage,
		Declared:        []staging.WorkspaceChange{{Path: "main.go", Action: "modify"}},
		QualityPassed:   true,
	})
	var promotionErr *Error
	if !errors.As(err, &promotionErr) || promotionErr.Kind != KindUnsafeConfig {
		t.Fatalf("err = %#v", err)
	}
}
