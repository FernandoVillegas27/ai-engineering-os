# Release readiness, evidence and remaining work

Checkpoint: 2026-11-06; base `master` commit `c73ac5eea28a7edaea285961a30a862af8da1d5d` (merged PR #134).

## Verified repository evidence

- PR #125: immutable archives with raw event hashes and snapshot linkage.
- PR #126: archive-backed reconstruction and explicitly verified compaction.
- PR #128: versioned artifact and coordination payload schemas.
- PR #130: structured Gradle/Maven/Jest/Vitest failures; Angular `ng test` deliberately not guaranteed structured.
- PR #131: deterministic OpenAPI compatibility and path-scoped forbidden-import checks.
- PR #132: repeated lease/heartbeat and long-history recovery tests.
- PR #133: bounded optional Engram recall and curated run learning.
- PR #134: normalized external-tool health; default bounded probe/operation deadlines.
- PR #134's head Go tests workflow completed successfully.
- No open PRs or issues were returned in the repository search at this checkpoint.

These facts establish milestones, NOT a production-ready or 100%-complete product. A green unit test workflow is not a production release certification.

## Next execution sequence (priority order)

### R1 — Threat model and negative security tests (release blocker)

Document and reproduce trust boundaries for untrusted project source, external model responses, command execution, tool/MCP output, persisted artifacts, event archives, snapshots, the original workspace and explicit promotion.

Tests must cover:
- symlink / junction / reparse-point escape and path traversal on supported operating systems;
- TOCTOU mutation between scope validation, staging verification and promotion;
- malicious repository test scripts, hostile provider output and prompt-injection text;
- secret or private-path leakage in diagnostics, events and tool health output;
- archive/snapshot tampering, missing sequence, interrupted compaction, recovery after crash;
- cancellation, timeout and process-tree termination;
- external tools that advertise health but fail authorization or transport.

Acceptance: every negative scenario is either blocked with typed evidence or explicitly excluded from autonomy; no fail-open result.

### R2 — Cross-platform integration pipeline (release blocker)

Add CI matrix for Linux and Windows, plus macOS if supported in release scope. Verify Go tests, race tests where supported, build, doctor behavior without optional providers, and deterministic live CLI integration in a temporary repository. Include Windows symlink privilege limitations, lock behavior, process cancellation and path case sensitivity.

Acceptance: reproducible green runs with downloadable diagnostics and no assumptions about local Cursor/Codex credentials.

### R3 — Real end-to-end coordinated execution (release blocker)

Execute one read-only and one staged-mutation scenario using multiple agents, typed handoffs, dependency scheduling, quality, independent review, promotion approval and recovery. Assert original-tree immutability until explicit promotion. Capture event log, graph, messages, artifacts and deterministic final summary.

Acceptance: scenarios pass from a fresh checkout; crash/restart does not silently drop state or duplicate promotion.

### R4 — Usage limits and provider fallback (release blocker for unattended mode)

Enforce per-run invocation/time/correction ceilings across model/runtime and external tool paths. Treat retryability as evidence, never implicit permission. Preserve Cursor auto / provider independence; no fixed model per role.

Acceptance: intentional quota exhaustion stops with a typed and inspectable terminal reason, without runaway calls.

### R5 — Memory governance (release blocker for unattended learning)

Add explicit review/delete/supersede lifecycle for stored project learning, sensitive-content redaction, retention policy, provenance and project boundary tests.

Acceptance: remembered data is inspectable and reversible, and never becomes an instruction/permission override.

### R6 — Developer-facing operations and adoption (release candidate)

Confirm actual supported IDE/runtime adapters rather than inferring support from roadmap text; add an installation quickstart, compatibility matrix, troubleshooting, migration guide and release versioning. An optional Control Plane UI can follow stable API integration, but is not a substitute for functional correctness.

Acceptance: fresh-user setup and diagnostic scenario work with documented prerequisites and no private maintainer knowledge.

## Release decision gate

| Gate | Evidence required | Status at checkpoint |
| --- | --- | --- |
| Foundation / typed collaboration | merged #125–#134 functionality | implemented, not independently audited end-to-end |
| Go unit CI | #134 head GitHub Actions successful | observed pass |
| Threat model + abuse tests | reviewed risks and executable negative tests | not established |
| Multi-OS integration | green matrix and runtime evidence | not established |
| Multi-agent live E2E | deterministic fresh-checkout evidence | not established |
| Budget / quota exhaustion | bounded runs with terminal evidence | not established |
| Memory deletion / supersession | user-controlled audited behavior | not established |
| Release docs and version | tagged reproducible release | not established |

**Decision: not production certified; do not claim 100%.** Complete release blockers before enabling broad unattended autonomy. This file is a roadmap and evidence ledger, not proof the unverified scenarios pass.
