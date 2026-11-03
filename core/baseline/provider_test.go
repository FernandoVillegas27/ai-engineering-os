package baseline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"syscall"
	"testing"

	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/staging"
	"github.com/FernandoVillegas27/ai-engineering-os/core/verification"
)

const (
	secretMainContent = "package main\n// TOP-SECRET-CONTENT-123\n"
	secretEnvContent  = "TOKEN=ENV-SECRET-456\n"
)

type fixture struct {
	base       string
	original   string
	staged     string
	preImage   staging.PreImage
	authorized []string
}

func writeFile(t *testing.T, root, relative, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// newFixture builds an original project, a staged workspace created from it through the
// same safe view the stage runtimes use, and simulates an agent changing two authorized files.
func newFixture(t *testing.T, prepare ...func(original string)) fixture {
	t.Helper()
	base := t.TempDir()
	original := filepath.Join(base, "project")
	staged := filepath.Join(base, "stage")

	for relative, content := range map[string]string{
		"main.go":                 secretMainContent,
		"lib/util.go":             "package lib\n",
		"README.md":               "# project\n",
		"go.mod":                  "module example.com/project\n\ngo 1.23\n",
		".env":                    secretEnvContent,
		"config/credentials.json": "{}",
		"assets/logo.png":         "png",
		"certs/server.pem":        "pem",
	} {
		writeFile(t, original, relative, content)
	}
	for _, hook := range prepare {
		hook(original)
	}

	if err := os.MkdirAll(staged, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := execution.PrepareSafeReadOnlyView(original, staged); err != nil {
		t.Fatal(err)
	}
	snapshot, err := staging.SnapshotWorkspace(staged, execution.DefaultSafeViewPolicy())
	if err != nil {
		t.Fatal(err)
	}
	fx := fixture{base: base, original: original, staged: staged, preImage: staging.NewPreImage(snapshot), authorized: []string{"main.go", "lib/new.go"}}

	writeFile(t, staged, "main.go", "package main\n// changed by the agent\n")
	writeFile(t, staged, "lib/new.go", "package lib\n")
	return fx
}

type fakeVerifier struct {
	mu         sync.Mutex
	calls      int
	workspaces []string
	report     verification.Report
	hook       func(ctx context.Context, workspace string) (verification.Report, error)
}

func (f *fakeVerifier) ID() string { return "fake-verifier" }

func (f *fakeVerifier) Verify(ctx context.Context, workspace string) (verification.Report, error) {
	f.mu.Lock()
	f.calls++
	f.workspaces = append(f.workspaces, workspace)
	hook := f.hook
	f.mu.Unlock()
	if hook != nil {
		return hook(ctx, workspace)
	}
	return f.report, nil
}

func (f *fakeVerifier) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func sampleReport() verification.Report {
	return verification.Report{Status: verification.StatusFailed, Checks: []verification.Check{{
		Name: "go-test", Command: []string{"go", "test", "-json", "./..."},
		FailureParse: verification.ParseComplete,
		Failures:     []verification.Failure{{ID: "example.com/project#TestOld", Class: verification.FailureClassTest, Signature: "sig"}},
	}}}
}

func (fx fixture) provider(t *testing.T, verifier verification.Runtime) *Provider {
	t.Helper()
	provider, err := New(Config{OriginalRoot: fx.original, PreImage: fx.preImage, Authorized: fx.authorized, Verifier: verifier})
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

func treeHash(t *testing.T, root string) string {
	t.Helper()
	var entries []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, _ := filepath.Rel(root, path)
		if entry.IsDir() {
			entries = append(entries, "d:"+filepath.ToSlash(relative))
			return nil
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			entries = append(entries, "l:"+filepath.ToSlash(relative))
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		entries = append(entries, "f:"+filepath.ToSlash(relative)+":"+hex.EncodeToString(sum[:]))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(entries)
	sum := sha256.Sum256([]byte(strings.Join(entries, "\n")))
	return hex.EncodeToString(sum[:])
}

func leftovers(t *testing.T, base string) []string {
	t.Helper()
	entries, err := os.ReadDir(base)
	if err != nil {
		t.Fatal(err)
	}
	var found []string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".ai-engineering-os-baseline-") {
			found = append(found, entry.Name())
		}
	}
	return found
}

func requireSymlink(t *testing.T, target, link string) {
	t.Helper()
	err := os.Symlink(target, link)
	if err == nil {
		return
	}
	if runtime.GOOS == "windows" && errors.Is(err, syscall.Errno(1314)) {
		t.Skipf("symlink creation requires a privilege this user does not hold: %v", err)
	}
	t.Fatal(err)
}

func requireIntegrity(t *testing.T, err error, kind Kind) *IntegrityError {
	t.Helper()
	if !errors.Is(err, ErrIntegrity) {
		t.Fatalf("expected an integrity error, got %v", err)
	}
	var integrityErr *IntegrityError
	if !errors.As(err, &integrityErr) || integrityErr.Kind != kind {
		t.Fatalf("expected kind %q, got %#v", kind, err)
	}
	return integrityErr
}

func TestBaselineVerifiesAnIdenticalCleanViewAndRemovesIt(t *testing.T) {
	fx := newFixture(t)
	originalBefore, stagedBefore := treeHash(t, fx.original), treeHash(t, fx.staged)

	verifier := &fakeVerifier{}
	verifier.hook = func(_ context.Context, workspace string) (verification.Report, error) {
		if workspace == fx.original || workspace == fx.staged {
			t.Errorf("baseline must run in its own directory, got %s", workspace)
		}
		if relative, err := filepath.Rel(fx.original, workspace); err == nil && !strings.HasPrefix(relative, "..") {
			t.Errorf("baseline directory %s must be outside the original project", workspace)
		}
		content, err := os.ReadFile(filepath.Join(workspace, "main.go"))
		if err != nil || string(content) != secretMainContent {
			t.Errorf("baseline must hold the original content, not the staged change: %q %v", content, err)
		}
		for _, excluded := range []string{".env", "config/credentials.json", "assets/logo.png", "certs/server.pem", "lib/new.go"} {
			if _, err := os.Stat(filepath.Join(workspace, filepath.FromSlash(excluded))); !os.IsNotExist(err) {
				t.Errorf("%s must not be present in the baseline view", excluded)
			}
		}
		if _, err := os.Stat(filepath.Join(workspace, "lib", "util.go")); err != nil {
			t.Errorf("allowed files must be present: %v", err)
		}
		return sampleReport(), nil
	}

	result, err := fx.provider(t, verifier).Baseline(context.Background(), fx.staged)
	if err != nil {
		t.Fatal(err)
	}
	if verifier.callCount() != 1 || len(result.Report.Checks) != 1 || result.Report.Checks[0].Failures[0].ID != "example.com/project#TestOld" {
		t.Fatalf("result=%#v calls=%d", result, verifier.callCount())
	}
	if result.PreImageDigest != fx.preImage.Digest() {
		t.Fatalf("digest=%q want %q", result.PreImageDigest, fx.preImage.Digest())
	}
	if strings.Join(result.StagedChanges, ",") != "lib/new.go,main.go" {
		t.Fatalf("staged changes=%v", result.StagedChanges)
	}
	if left := leftovers(t, fx.base); len(left) != 0 {
		t.Fatalf("temporary baseline directories left behind: %v", left)
	}
	if treeHash(t, fx.original) != originalBefore || treeHash(t, fx.staged) != stagedBefore {
		t.Fatal("the baseline must not modify the original project or the staged workspace")
	}
}

func TestBaselineRejectsAnOriginalThatChangedBeforeTheBaseline(t *testing.T) {
	cases := map[string]struct {
		change func(t *testing.T, original string)
		path   string
	}{
		"authorized file modified": {func(t *testing.T, o string) { writeFile(t, o, "main.go", "package main\n// edited later\n") }, "main.go"},
		"unauthorized file modified": {func(t *testing.T, o string) { writeFile(t, o, "lib/util.go", "package lib\n// edited\n") }, "lib/util.go"},
		"file added":                 {func(t *testing.T, o string) { writeFile(t, o, "extra.go", "package main\n") }, "extra.go"},
		"file deleted": {func(t *testing.T, o string) {
			if err := os.Remove(filepath.Join(o, "README.md")); err != nil {
				t.Fatal(err)
			}
		}, "README.md"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			fx := newFixture(t)
			tc.change(t, fx.original)
			originalAfterChange := treeHash(t, fx.original)

			verifier := &fakeVerifier{report: sampleReport()}
			_, err := fx.provider(t, verifier).Baseline(context.Background(), fx.staged)

			got := requireIntegrity(t, err, KindOriginalChanged)
			if len(got.Paths) != 1 || got.Paths[0] != tc.path {
				t.Fatalf("paths=%v want [%s]", got.Paths, tc.path)
			}
			if verifier.callCount() != 0 {
				t.Fatal("the verifier must not run on a baseline that fails integrity")
			}
			if left := leftovers(t, fx.base); len(left) != 0 {
				t.Fatalf("leftovers=%v", left)
			}
			if treeHash(t, fx.original) != originalAfterChange {
				t.Fatal("the provider must never modify the original project")
			}
		})
	}
}

func TestBaselineRejectsUnauthorizedChangesInTheStagedWorkspace(t *testing.T) {
	cases := map[string]struct {
		change func(t *testing.T, staged string)
		kind   Kind
	}{
		"unauthorized file modified": {func(t *testing.T, s string) { writeFile(t, s, "lib/util.go", "package lib\n// sneaky\n") }, KindStagedUnauthorized},
		"unauthorized file created":  {func(t *testing.T, s string) { writeFile(t, s, "notes.md", "notes") }, KindStagedUnauthorized},
		"unauthorized file deleted": {func(t *testing.T, s string) {
			if err := os.Remove(filepath.Join(s, "README.md")); err != nil {
				t.Fatal(err)
			}
		}, KindStagedUnauthorized},
		"sensitive file created":        {func(t *testing.T, s string) { writeFile(t, s, ".env", "TOKEN=x") }, KindStagedInvalid},
		"unauthorized file type":        {func(t *testing.T, s string) { writeFile(t, s, "payload.bin", "x") }, KindStagedInvalid},
		"sensitive directory created":   {func(t *testing.T, s string) { writeFile(t, s, ".git/config", "[core]") }, KindStagedInvalid},
		"authorized path with bad type": {func(t *testing.T, s string) { writeFile(t, s, "lib/new.go/inner.go", "x") }, KindStagedUnauthorized},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			fx := newFixture(t)
			if name == "authorized path with bad type" {
				// lib/new.go becomes a directory: an authorized name must not hide a different entry type.
				if err := os.Remove(filepath.Join(fx.staged, "lib", "new.go")); err != nil {
					t.Fatal(err)
				}
			}
			tc.change(t, fx.staged)

			verifier := &fakeVerifier{report: sampleReport()}
			_, err := fx.provider(t, verifier).Baseline(context.Background(), fx.staged)

			requireIntegrity(t, err, tc.kind)
			if verifier.callCount() != 0 {
				t.Fatal("no verification may run when the staged workspace fails integrity")
			}
			if left := leftovers(t, fx.base); len(left) != 0 {
				t.Fatalf("leftovers=%v", left)
			}
		})
	}
}

func TestBaselineRejectsASymlinkInTheStagedWorkspace(t *testing.T) {
	fx := newFixture(t)
	requireSymlink(t, filepath.Join(fx.staged, "main.go"), filepath.Join(fx.staged, "link.go"))

	_, err := fx.provider(t, &fakeVerifier{report: sampleReport()}).Baseline(context.Background(), fx.staged)
	requireIntegrity(t, err, KindStagedInvalid)
}

func TestBaselineExcludesSymlinksAndSensitiveFilesOfTheOriginal(t *testing.T) {
	fx := newFixture(t, func(original string) {
		requireSymlink(t, filepath.Join(original, ".env"), filepath.Join(original, "linked-secret.go"))
	})
	verifier := &fakeVerifier{report: sampleReport()}
	verifier.hook = func(_ context.Context, workspace string) (verification.Report, error) {
		if _, err := os.Lstat(filepath.Join(workspace, "linked-secret.go")); !os.IsNotExist(err) {
			t.Error("a symlink of the original must not reach the baseline view")
		}
		return sampleReport(), nil
	}
	if _, err := fx.provider(t, verifier).Baseline(context.Background(), fx.staged); err != nil {
		t.Fatal(err)
	}
}

func TestBaselineRejectsUsingTheOriginalAsTheStagedWorkspace(t *testing.T) {
	fx := newFixture(t)
	_, err := fx.provider(t, &fakeVerifier{report: sampleReport()}).Baseline(context.Background(), fx.original)
	requireIntegrity(t, err, KindUnsafeConfig)

	_, err = fx.provider(t, &fakeVerifier{report: sampleReport()}).Baseline(context.Background(), filepath.Join(fx.base, "missing"))
	requireIntegrity(t, err, KindStagedInvalid)
}

func TestBaselineIsMemoizedOnlyWhileIntegrityHolds(t *testing.T) {
	fx := newFixture(t)
	verifier := &fakeVerifier{report: sampleReport()}
	provider := fx.provider(t, verifier)
	ctx := context.Background()

	first, err := provider.Baseline(ctx, fx.staged)
	if err != nil {
		t.Fatal(err)
	}
	first.Report.Checks[0].Name = "tampered"
	first.Report.Checks[0].Failures[0].ID = "tampered"

	// Reuse is allowed while the staged workspace stays within the authorized files.
	writeFile(t, fx.staged, "main.go", "package main\n// second correction\n")
	second, err := provider.Baseline(ctx, fx.staged)
	if err != nil {
		t.Fatal(err)
	}
	if verifier.callCount() != 1 {
		t.Fatalf("expected one verification, got %d", verifier.callCount())
	}
	if second.Report.Checks[0].Name != "go-test" || second.Report.Checks[0].Failures[0].ID != "example.com/project#TestOld" {
		t.Fatalf("a returned report must not alias the memoized one: %#v", second.Report.Checks[0])
	}

	// An unauthorized staged change blocks reuse without discarding a still-valid baseline.
	writeFile(t, fx.staged, "lib/util.go", "package lib\n// sneaky\n")
	_, err = provider.Baseline(ctx, fx.staged)
	requireIntegrity(t, err, KindStagedUnauthorized)
	writeFile(t, fx.staged, "lib/util.go", "package lib\n")
	if _, err := provider.Baseline(ctx, fx.staged); err != nil || verifier.callCount() != 1 {
		t.Fatalf("err=%v calls=%d", err, verifier.callCount())
	}

	// A changed original invalidates the baseline for good.
	writeFile(t, fx.original, "lib/util.go", "package lib\n// original moved on\n")
	_, err = provider.Baseline(ctx, fx.staged)
	requireIntegrity(t, err, KindOriginalChanged)
	writeFile(t, fx.original, "lib/util.go", "package lib\n")
	if _, err := provider.Baseline(ctx, fx.staged); err != nil {
		t.Fatal(err)
	}
	if verifier.callCount() != 2 {
		t.Fatalf("an invalidated baseline must be recomputed, calls=%d", verifier.callCount())
	}
}

func TestBaselineIsNotMemoizedWhenTheOriginalChangesDuringVerification(t *testing.T) {
	fx := newFixture(t)
	verifier := &fakeVerifier{}
	mutate := true
	verifier.hook = func(_ context.Context, _ string) (verification.Report, error) {
		if mutate {
			writeFile(t, fx.original, "lib/util.go", "package lib\n// changed mid-run\n")
		}
		return sampleReport(), nil
	}
	provider := fx.provider(t, verifier)

	_, err := provider.Baseline(context.Background(), fx.staged)
	requireIntegrity(t, err, KindOriginalChanged)
	if left := leftovers(t, fx.base); len(left) != 0 {
		t.Fatalf("leftovers=%v", left)
	}

	mutate = false
	writeFile(t, fx.original, "lib/util.go", "package lib\n")
	if _, err := provider.Baseline(context.Background(), fx.staged); err != nil {
		t.Fatal(err)
	}
	if verifier.callCount() != 2 {
		t.Fatalf("a baseline taken during a change must not have been stored, calls=%d", verifier.callCount())
	}
}

func TestBaselineHonorsContextCancellation(t *testing.T) {
	t.Run("already canceled", func(t *testing.T) {
		fx := newFixture(t)
		verifier := &fakeVerifier{report: sampleReport()}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := fx.provider(t, verifier).Baseline(ctx, fx.staged); !errors.Is(err, context.Canceled) {
			t.Fatalf("err=%v", err)
		}
		if verifier.callCount() != 0 || len(leftovers(t, fx.base)) != 0 {
			t.Fatal("a canceled call must neither verify nor leave directories")
		}
	})

	t.Run("canceled during verification", func(t *testing.T) {
		fx := newFixture(t)
		ctx, cancel := context.WithCancel(context.Background())
		verifier := &fakeVerifier{}
		verifier.hook = func(ctx context.Context, _ string) (verification.Report, error) {
			cancel()
			<-ctx.Done()
			return verification.Report{}, ctx.Err()
		}
		provider := fx.provider(t, verifier)

		if _, err := provider.Baseline(ctx, fx.staged); !errors.Is(err, context.Canceled) || errors.Is(err, ErrIntegrity) {
			t.Fatalf("err=%v", err)
		}
		if left := leftovers(t, fx.base); len(left) != 0 {
			t.Fatalf("leftovers=%v", left)
		}

		verifier.hook = nil
		verifier.report = sampleReport()
		if _, err := provider.Baseline(context.Background(), fx.staged); err != nil || verifier.callCount() != 2 {
			t.Fatalf("a canceled run must not be memoized: err=%v calls=%d", err, verifier.callCount())
		}
	})
}

func TestBaselineReportsVerifierFailuresAndCleansUp(t *testing.T) {
	fx := newFixture(t)
	verifier := &fakeVerifier{hook: func(context.Context, string) (verification.Report, error) {
		return verification.Report{}, fmt.Errorf("verifier exploded")
	}}
	provider := fx.provider(t, verifier)

	_, err := provider.Baseline(context.Background(), fx.staged)
	if err == nil || errors.Is(err, ErrIntegrity) || !strings.Contains(err.Error(), "verifier exploded") {
		t.Fatalf("err=%v", err)
	}
	if left := leftovers(t, fx.base); len(left) != 0 {
		t.Fatalf("leftovers=%v", left)
	}
	verifier.hook = nil
	verifier.report = sampleReport()
	if _, err := provider.Baseline(context.Background(), fx.staged); err != nil || verifier.callCount() != 2 {
		t.Fatalf("err=%v calls=%d", err, verifier.callCount())
	}
}

func TestBaselineSerializesConcurrentCallers(t *testing.T) {
	fx := newFixture(t)
	verifier := &fakeVerifier{report: sampleReport()}
	provider := fx.provider(t, verifier)

	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := provider.Baseline(context.Background(), fx.staged)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if verifier.callCount() != 1 {
		t.Fatalf("concurrent callers must share one verification, got %d", verifier.callCount())
	}
}

func TestNewValidatesTheConfiguration(t *testing.T) {
	fx := newFixture(t)
	verifier := &fakeVerifier{}

	if _, err := New(Config{OriginalRoot: fx.original, PreImage: fx.preImage, Authorized: fx.authorized}); err == nil {
		t.Fatal("a verifier is required")
	}
	_, err := New(Config{OriginalRoot: fx.original, Authorized: fx.authorized, Verifier: verifier})
	requireIntegrity(t, err, KindPreImageMissing)

	_, err = New(Config{OriginalRoot: fx.original, PreImage: fx.preImage, Authorized: fx.authorized, Verifier: verifier, TempParent: filepath.Join(fx.original, "lib")})
	requireIntegrity(t, err, KindUnsafeConfig)
	_, err = New(Config{OriginalRoot: fx.original, PreImage: fx.preImage, Authorized: fx.authorized, Verifier: verifier, TempParent: fx.original})
	requireIntegrity(t, err, KindUnsafeConfig)

	for _, bad := range []string{"../escape.go", ".env", "logo.png", "/abs.go", ""} {
		if _, err := New(Config{OriginalRoot: fx.original, PreImage: fx.preImage, Authorized: []string{bad}, Verifier: verifier}); err == nil {
			t.Fatalf("authorized path %q must be rejected", bad)
		}
	}
	if _, err := New(Config{OriginalRoot: filepath.Join(fx.base, "missing"), PreImage: fx.preImage, Verifier: verifier}); err == nil {
		t.Fatal("the original project must exist")
	}
}

func TestProviderIsNotAffectedByLaterChangesToTheSnapshotItWasBuiltFrom(t *testing.T) {
	fx := newFixture(t)
	snapshot := fx.preImage.Snapshot()
	preImage := staging.NewPreImage(snapshot)
	for path := range snapshot {
		delete(snapshot, path)
	}
	view := preImage.Snapshot()
	view["main.go"] = [32]byte{1}

	provider, err := New(Config{OriginalRoot: fx.original, PreImage: preImage, Authorized: fx.authorized, Verifier: &fakeVerifier{report: sampleReport()}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Baseline(context.Background(), fx.staged); err != nil {
		t.Fatalf("a tampered copy must not reach the provider: %v", err)
	}
}

func TestIntegrityErrorsNeverExposeContentsHashesOrLocations(t *testing.T) {
	hexHash := regexp.MustCompile(`[0-9a-f]{64}`)
	scenarios := map[string]func(t *testing.T, fx fixture) error{
		"original changed": func(t *testing.T, fx fixture) error {
			writeFile(t, fx.original, "main.go", "package main\n// ANOTHER-SECRET-789\n")
			_, err := fx.provider(t, &fakeVerifier{}).Baseline(context.Background(), fx.staged)
			return err
		},
		"unauthorized staged change": func(t *testing.T, fx fixture) error {
			writeFile(t, fx.staged, "lib/util.go", "package lib\n// STAGED-SECRET-321\n")
			_, err := fx.provider(t, &fakeVerifier{}).Baseline(context.Background(), fx.staged)
			return err
		},
		"sensitive staged file": func(t *testing.T, fx fixture) error {
			writeFile(t, fx.staged, ".env", "TOKEN=LEAK-654")
			_, err := fx.provider(t, &fakeVerifier{}).Baseline(context.Background(), fx.staged)
			return err
		},
	}
	for name, run := range scenarios {
		t.Run(name, func(t *testing.T) {
			fx := newFixture(t)
			err := run(t, fx)
			if err == nil {
				t.Fatal("expected an error")
			}
			message := err.Error()
			for _, forbidden := range []string{"TOP-SECRET-CONTENT-123", "ENV-SECRET-456", "ANOTHER-SECRET-789", "STAGED-SECRET-321", "LEAK-654", fx.base} {
				if strings.Contains(message, forbidden) {
					t.Fatalf("error leaks %q: %s", forbidden, message)
				}
			}
			if hexHash.MatchString(message) {
				t.Fatalf("error must not contain file hashes: %s", message)
			}
		})
	}
}

func TestIntegrityErrorListsAtMostTenPathsButKeepsAllOfThem(t *testing.T) {
	fx := newFixture(t)
	for i := 0; i < 25; i++ {
		writeFile(t, fx.staged, fmt.Sprintf("extra/file%02d.go", i), "package extra\n")
	}
	_, err := fx.provider(t, &fakeVerifier{}).Baseline(context.Background(), fx.staged)
	got := requireIntegrity(t, err, KindStagedUnauthorized)
	if len(got.Paths) != 25 || !strings.Contains(got.Error(), "(+15 more)") {
		t.Fatalf("paths=%d message=%s", len(got.Paths), got.Error())
	}
	if strings.Count(got.Error(), "extra/file") != maxListedPaths {
		t.Fatalf("message must list exactly %d paths: %s", maxListedPaths, got.Error())
	}
}
