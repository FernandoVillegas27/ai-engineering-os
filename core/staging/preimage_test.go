package staging

import (
	"crypto/sha256"
	"reflect"
	"testing"
)

func hashOf(content string) [32]byte { return sha256.Sum256([]byte(content)) }

func sampleSnapshot() WorkspaceSnapshot {
	return WorkspaceSnapshot{
		"main.go":     hashOf("package main"),
		"lib/util.go": hashOf("package lib"),
		"README.md":   hashOf("# readme"),
	}
}

func TestSnapshotDigestIsStableAndOrderIndependent(t *testing.T) {
	first := sampleSnapshot()
	second := WorkspaceSnapshot{}
	for _, path := range []string{"README.md", "lib/util.go", "main.go"} {
		second[path] = first[path]
	}
	if first.Digest() != second.Digest() {
		t.Fatal("digest must not depend on map insertion order")
	}
	if got := first.Digest(); len(got) != 64 || got != first.Digest() {
		t.Fatalf("digest=%q", got)
	}
}

func TestSnapshotDigestChangesWithPathContentOrMembership(t *testing.T) {
	base := sampleSnapshot().Digest()
	mutations := map[string]func(WorkspaceSnapshot){
		"content changed": func(s WorkspaceSnapshot) { s["main.go"] = hashOf("package other") },
		"file added":      func(s WorkspaceSnapshot) { s["extra.go"] = hashOf("x") },
		"file removed":    func(s WorkspaceSnapshot) { delete(s, "README.md") },
		"file renamed": func(s WorkspaceSnapshot) {
			s["main2.go"] = s["main.go"]
			delete(s, "main.go")
		},
	}
	for name, mutate := range mutations {
		snapshot := sampleSnapshot()
		mutate(snapshot)
		if snapshot.Digest() == base {
			t.Fatalf("%s must change the digest", name)
		}
	}
	if (WorkspaceSnapshot{}).Digest() == base {
		t.Fatal("empty snapshot must differ")
	}
}

func TestSnapshotDigestFramingIsUnambiguous(t *testing.T) {
	// A path that embeds a newline must not be confusable with two separate entries.
	a := WorkspaceSnapshot{"a": hashOf("1")}
	b := WorkspaceSnapshot{"a\n": hashOf("1")}
	if a.Digest() == b.Digest() {
		t.Fatal("different paths must produce different digests")
	}
}

func TestPreImageIsImmutableThroughDefensiveCopies(t *testing.T) {
	source := sampleSnapshot()
	preImage := NewPreImage(source)
	digest := preImage.Digest()

	source["main.go"] = hashOf("tampered")
	source["new.go"] = hashOf("new")
	delete(source, "README.md")
	if preImage.Digest() != digest || preImage.Len() != 3 {
		t.Fatal("changing the source map altered the pre-image")
	}

	view := preImage.Snapshot()
	view["main.go"] = hashOf("tampered via accessor")
	delete(view, "lib/util.go")
	if !reflect.DeepEqual(preImage.Snapshot(), sampleSnapshot()) || preImage.Snapshot().Digest() != digest {
		t.Fatal("changing a returned snapshot altered the pre-image")
	}
}

func TestPreImageZeroValueIsDistinctFromEmptyProject(t *testing.T) {
	if !(PreImage{}).IsZero() {
		t.Fatal("zero value must report IsZero")
	}
	empty := NewPreImage(WorkspaceSnapshot{})
	if empty.IsZero() || empty.Len() != 0 || empty.Digest() == "" {
		t.Fatalf("empty=%#v", empty)
	}
	if NewPreImage(nil).IsZero() {
		t.Fatal("a recorded nil snapshot is an empty pre-image, not a missing one")
	}
}

func TestDiffPathsReportsPathsOnly(t *testing.T) {
	preImage := NewPreImage(sampleSnapshot())
	other := sampleSnapshot()
	other["main.go"] = hashOf("changed")
	other["added.go"] = hashOf("added")
	delete(other, "README.md")

	want := []string{"README.md", "added.go", "main.go"}
	if got := preImage.DiffPaths(other); !reflect.DeepEqual(got, want) {
		t.Fatalf("diff=%v want %v", got, want)
	}
	if got := preImage.DiffPaths(sampleSnapshot()); len(got) != 0 {
		t.Fatalf("identical snapshots must have no diff, got %v", got)
	}
}
