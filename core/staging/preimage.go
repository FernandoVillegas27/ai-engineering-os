package staging

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
)

const preImageDigestHeader = "ai-engineering-os preimage v1\n"

// Clone returns an independent copy of the snapshot.
func (s WorkspaceSnapshot) Clone() WorkspaceSnapshot {
	if s == nil {
		return nil
	}
	clone := make(WorkspaceSnapshot, len(s))
	for path, hash := range s {
		clone[path] = hash
	}
	return clone
}

// Digest is a deterministic SHA-256 over the sorted paths and file hashes.
// Each entry is framed as path NUL hex(hash) LF; the hash has a fixed width, so
// the framing is unambiguous even for unusual file names.
func (s WorkspaceSnapshot) Digest() string {
	paths := make([]string, 0, len(s))
	for path := range s {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	hash := sha256.New()
	hash.Write([]byte(preImageDigestHeader))
	for _, path := range paths {
		entry := s[path]
		hash.Write([]byte(path))
		hash.Write([]byte{0})
		hash.Write([]byte(hex.EncodeToString(entry[:])))
		hash.Write([]byte{'\n'})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

// DiffPaths lists, sorted, every path whose presence or content differs.
// It reports paths only, never contents or hashes.
func (s WorkspaceSnapshot) DiffPaths(other WorkspaceSnapshot) []string {
	diff := []string{}
	for path, hash := range s {
		if otherHash, exists := other[path]; !exists || otherHash != hash {
			diff = append(diff, path)
		}
	}
	for path := range other {
		if _, exists := s[path]; !exists {
			diff = append(diff, path)
		}
	}
	sort.Strings(diff)
	return diff
}

// PreImage is the immutable record of a workspace before an agent touched it.
// Its fields are unexported and every accessor copies, so a held PreImage
// cannot be altered through the map it was built from or through a returned view.
type PreImage struct {
	entries WorkspaceSnapshot
	digest  string
}

// NewPreImage copies the snapshot so later changes to it cannot alter the PreImage.
func NewPreImage(snapshot WorkspaceSnapshot) PreImage {
	entries := snapshot.Clone()
	if entries == nil {
		entries = WorkspaceSnapshot{}
	}
	return PreImage{entries: entries, digest: entries.Digest()}
}

// IsZero reports that no pre-image was recorded.
func (p PreImage) IsZero() bool { return p.entries == nil }

func (p PreImage) Len() int { return len(p.entries) }

func (p PreImage) Digest() string { return p.digest }

// Snapshot returns an independent copy of the recorded hashes.
func (p PreImage) Snapshot() WorkspaceSnapshot { return p.entries.Clone() }

// DiffPaths lists the paths in which other differs from the recorded state.
func (p PreImage) DiffPaths(other WorkspaceSnapshot) []string {
	return p.entries.DiffPaths(other)
}
