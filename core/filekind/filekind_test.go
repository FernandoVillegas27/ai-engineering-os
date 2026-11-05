package filekind

import (
	"testing"

	"github.com/FernandoVillegas27/ai-engineering-os/core/staging"
)

func TestClassifyRecognizesConservativeTestPaths(t *testing.T) {
	cases := map[string]Kind{
		"pkg/service_test.go":                 KindTest,
		"tests/service.py":                    KindTest,
		"src/test/java/com/acme/FooTest.java": KindTest,
		"src/__tests__/app.ts":                KindTest,
		"src/app.spec.ts":                     KindTest,
		"src/app.test.tsx":                    KindTest,
		"test_service.py":                     KindTest,
		"spec/api/client.js":                  KindTest,
		"integration-tests/api_test.go":       KindTest,
	}
	for path, want := range cases {
		got, err := Classify(path)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		if got != want {
			t.Fatalf("%s: got %s want %s", path, got, want)
		}
	}
}

func TestClassifyDoesNotGuessAmbiguousSourceFilesAsTests(t *testing.T) {
	cases := map[string]Kind{
		"src/main/java/com/acme/FooTest.java": KindSource,
		"src/testing/helpers.ts":              KindSource,
		"src/testimony/service.go":            KindSource,
		"src/service.ts":                      KindSource,
		"src/service.py":                      KindSource,
		"README.md":                           KindUnknown,
		"fixtures/data.json":                  KindUnknown,
	}
	for path, want := range cases {
		got, err := Classify(path)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		if got != want {
			t.Fatalf("%s: got %s want %s", path, got, want)
		}
	}
}

func TestClassifyRejectsUnsafePaths(t *testing.T) {
	cases := []string{
		"",
		"../outside_test.go",
		"../../test/app.spec.ts",
		"/absolute/test.go",
	}
	for _, path := range cases {
		if _, err := Classify(path); err == nil {
			t.Fatalf("expected unsafe path rejection for %q", path)
		}
	}
}

func TestValidateTestsOnlyAcceptsOnlyClearlyClassifiedTests(t *testing.T) {
	changes := []staging.WorkspaceChange{
		{Path: "pkg/service_test.go", Action: "modify"},
		{Path: "src/test/java/com/acme/FooTest.java", Action: "create"},
		{Path: "frontend/app.spec.ts", Action: "create"},
	}
	if err := ValidateTestsOnly(changes); err != nil {
		t.Fatal(err)
	}
}

func TestValidateTestsOnlyRejectsSourceAndUnknownPaths(t *testing.T) {
	cases := [][]staging.WorkspaceChange{
		{{Path: "src/service.go", Action: "modify"}},
		{{Path: "src/main/java/com/acme/FooTest.java", Action: "create"}},
		{{Path: "fixtures/data.json", Action: "create"}},
		{},
	}
	for _, changes := range cases {
		if err := ValidateTestsOnly(changes); err == nil {
			t.Fatalf("expected tests-only rejection for %#v", changes)
		}
	}
}

func TestValidateTestsOnlyRejectsMixedScope(t *testing.T) {
	err := ValidateTestsOnly([]staging.WorkspaceChange{
		{Path: "pkg/service_test.go", Action: "modify"},
		{Path: "pkg/service.go", Action: "modify"},
	})
	if err == nil {
		t.Fatal("expected mixed test/source scope rejection")
	}
}
