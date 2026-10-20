package staging

import (
	"strings"
	"testing"

	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
	"github.com/FernandoVillegas27/ai-engineering-os/core/proposal"
)

func TestValidateProposalAcceptsMatchingSafePatch(t *testing.T) {
	p := proposal.Proposal{
		Files: []proposal.FileChange{
			{Path: "src/service.go", Action: "modify"},
			{Path: "src/new.go", Action: "create"},
		},
		Patch: strings.Join([]string{
			"diff --git a/src/service.go b/src/service.go",
			"--- a/src/service.go",
			"+++ b/src/service.go",
			"@@ -1 +1 @@",
			"-old",
			"+new",
			"diff --git a/src/new.go b/src/new.go",
			"new file mode 100644",
			"--- /dev/null",
			"+++ b/src/new.go",
			"@@ -0,0 +1 @@",
			"+package src",
		}, "\n"),
	}

	files, err := ValidateProposal(p, execution.DefaultSafeViewPolicy())
	if err != nil {
		t.Fatalf("ValidateProposal() error = %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("files = %#v", files)
	}
}

func TestValidateProposalRejectsTraversal(t *testing.T) {
	p := proposal.Proposal{
		Files: []proposal.FileChange{{Path: "../outside.go", Action: "modify"}},
		Patch: "diff --git a/../outside.go b/../outside.go\n--- a/../outside.go\n+++ b/../outside.go\n@@ -1 +1 @@\n-a\n+b",
	}
	if _, err := ValidateProposal(p, execution.DefaultSafeViewPolicy()); err == nil {
		t.Fatal("expected traversal rejection")
	}
}

func TestValidateProposalRejectsSensitivePath(t *testing.T) {
	p := proposal.Proposal{
		Files: []proposal.FileChange{{Path: ".env", Action: "modify"}},
		Patch: "diff --git a/.env b/.env\n--- a/.env\n+++ b/.env\n@@ -1 +1 @@\n-a\n+b",
	}
	if _, err := ValidateProposal(p, execution.DefaultSafeViewPolicy()); err == nil {
		t.Fatal("expected sensitive-path rejection")
	}
}

func TestValidateProposalRejectsUndeclaredPatchFile(t *testing.T) {
	p := proposal.Proposal{
		Files: []proposal.FileChange{{Path: "src/a.go", Action: "modify"}},
		Patch: "diff --git a/src/b.go b/src/b.go\n--- a/src/b.go\n+++ b/src/b.go\n@@ -1 +1 @@\n-a\n+b",
	}
	if _, err := ValidateProposal(p, execution.DefaultSafeViewPolicy()); err == nil {
		t.Fatal("expected file mismatch rejection")
	}
}

func TestValidateProposalRejectsRename(t *testing.T) {
	p := proposal.Proposal{
		Files: []proposal.FileChange{{Path: "src/b.go", Action: "modify"}},
		Patch: "diff --git a/src/a.go b/src/b.go\n--- a/src/a.go\n+++ b/src/b.go",
	}
	if _, err := ValidateProposal(p, execution.DefaultSafeViewPolicy()); err == nil {
		t.Fatal("expected rename rejection")
	}
}
