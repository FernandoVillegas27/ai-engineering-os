# Release readiness, evidence and remaining work

Checkpoint: 2026-11-07; base `master` commit `cd819c783860b70df50159d84c48dd35f95418a8` (merged PR #152).

## Release-candidate scope

The current release line is:

~~~text
0.1.0-rc.1
~~~

For this release candidate, "ready" means the repository has executable evidence for:

1. deterministic/provider-independent multi-agent coordination;
2. durable state, recovery, snapshots, archives and compaction;
3. isolated staged mutation and explicit promotion approval;
4. baseline-aware Quality, independent review and contract/architecture gates;
5. bounded provider usage and fallback;
6. governed reversible project memory;
7. local Control Plane API/UI;
8. Linux/Windows/macOS verification;
9. release threat boundaries and explicit autonomy exclusions;
10. reproducible multi-platform packages with SHA-256 evidence;
11. fresh-user operational/versioning/migration documentation.

It does **not** mean the long-term product vision is exhausted. Dedicated VS Code/IntelliJ clients and broader ecosystem/language coverage remain future product evolution.

## Verified repository evidence

### Foundation and durable coordination

- #125: immutable archives with exact historical event bytes and snapshot linkage.
- #126: archive-backed reconstruction and explicit verified event compaction.
- #128: typed/versioned lifecycle artifact and coordination payload contracts.
- #130: structured Java/Gradle/Maven and Jest/Vitest failure identity.
- #131: deterministic OpenAPI and architecture compatibility gate.
- #132: long-run lease/recovery/event-history soak.
- #133: bounded project recall and curated team learning.
- #134: normalized external tool/MCP health and bounded deadlines.

### Release hardening

- #138 — merge `4dbbc900e57c6f8fbf724d79260e2406adab4797`
  - embedded read-only Control Plane web client;
  - same durable Control Plane remains source of truth.

- #139 — merge `c7082df278a7ca795a533a4c982fb4caefd4fafe`
  - promotion adversarial boundaries;
  - Linux/Windows/macOS CI matrix;
  - original-tree identity/preimage protection.

- #140 — merge `aaf262905c44bf2f84da63b906f13a555860847d`
  - coordinated multi-agent/promotion E2E;
  - original tree remains unchanged until explicit promotion.

- #142 — merge `32bea23bc25d8aa4a515b7a4e422c2ca4193e06e`
  - provider-independent invocation/runtime budgets;
  - bounded fallback and terminal quota evidence.

- #143 — merge `616b7076d43b4ba5fb33f18feaef3e09dc412915`
  - governed local memory authority;
  - provenance, retention, redaction, delete/supersede;
  - project isolation;
  - external memory provider reduced to optional mirror.

- #145 — merge `3fc887cd5654ba2b6a7532accc690c17a64f2cd6`
  - `ai version`;
  - quickstart;
  - evidence-based compatibility matrix;
  - troubleshooting, migration and release process.

- #146 — merge `aecdecdcd6a14c2edf6c162d7f7bb6bde09e07f3`
  - release threat hardening;
  - full process-tree cancellation;
  - diagnostic secret redaction;
  - prompt-injection trust-boundary tests;
  - traversal/absolute/drive/UNC/reparse-point promotion hardening;
  - explicit `--trust-project-code` for mutating repository verification;
  - isolated HOME/GOPATH/npm/Gradle/Maven verification environment.
  - complete Linux/Windows/macOS matrix passed, including Linux `go test -race ./...`.

- #149 — merge `d1f93b13725c83e7446d93d57e1f24a9258e8c1b`
  - fixed a real lease shutdown race found by release CI;
  - Stop now waits for heartbeat-loop completion before deleting the lease;
  - Linux/Windows/macOS matrix passed, including Linux race tests.

- #150 — merge `01a636b88810814bc2f686ded6913489450d9c7f`
  - reproducible RC package builder;
  - release-artifact workflow;
  - successful PR builds for:
    - linux/amd64;
    - windows/amd64;
    - darwin/amd64;
    - darwin/arm64;
  - SHA-256 file per package;
  - tag-only GitHub release publication path;
  - publication job correctly skipped during PR validation.
  - normal Linux/Windows/macOS matrix passed; Linux race tests passed.

- #152 — merge `cd819c783860b70df50159d84c48dd35f95418a8`
  - fixed coordinator cancellation classification exposed by the final Linux race gate;
  - worker `context.Canceled` / `context.DeadlineExceeded` caused by a cancelled run is now recorded as cancelled rather than failed;
  - 24-iteration cancellation stress regression test;
  - Linux/Windows/macOS matrix passed;
  - Linux `go test -race ./...` passed.

## Release blockers R1–R6

### R1 — Threat model and negative security tests

**Status: satisfied for the documented RC autonomy scope.**

Evidence:

- [THREAT_MODEL.md](THREAT_MODEL.md) maps threats to executable tests or explicit exclusions.
- hostile stage/promotion paths fail closed;
- Unix symlinks and Windows junction/reparse points are rejected;
- original-tree TOCTOU/preimage changes reject promotion;
- provider/repository prompt text remains untrusted evidence;
- external diagnostic secrets are redacted;
- archive/snapshot tampering fails closed;
- provider/test child process trees are terminated on cancellation;
- fallback/retry budgets are bounded;
- governed memory cannot silently revive deleted/superseded/expired knowledge;
- promotion always requires explicit approval.

Explicit release-scope restriction:

> Arbitrary untrusted repository test/build code is outside unattended mutating scope.

A mutating run requires the human operator to review/trust the project and supply `--trust-project-code`. That trust evidence is durable across pause/resume. The decision does not grant model/tool/promotion permissions.

This is a documented autonomy exclusion, not a claim of a universal third-party code sandbox.

### R2 — Cross-platform integration pipeline

**Status: satisfied.**

GitHub Actions verifies:

- Ubuntu;
- Windows;
- macOS.

Every normal matrix job runs:

- `go test ./...`;
- `go build ./...`;
- `go vet ./...`;
- `ai doctor` smoke;
- `ai version --json` smoke.

Linux additionally runs:

- `go test -race ./...`.

Release hardening PRs #146 and #149 passed the complete matrix.

### R3 — Real end-to-end coordinated execution

**Status: satisfied for deterministic release E2E.**

PR #140 exercises coordinated multi-agent scheduling/handoffs, Quality/review, promotion candidate and explicit promotion while preserving original-tree immutability before approval.

Durable recovery/long-history behavior is additionally exercised by #132.

### R4 — Usage limits and provider fallback

**Status: satisfied.**

PR #142 enforces shared per-run invocation/runtime limits across provider attempts.

Retryability remains evidence only; no infinite fallback loop is authorized.

Provider/model policy remains provider-independent. No model is hard-coded per agent role.

### R5 — Memory governance

**Status: satisfied.**

PR #143 adds:

- inspectable local governed memory;
- provenance;
- retention;
- secret redaction;
- project isolation;
- delete tombstones;
- supersession;
- stable-topic convergence;
- recall only from active governed records.

Engram remains an optional mirror rather than recall authority.

### R6 — Developer-facing operations and adoption

**Status: satisfied for RC scope.**

PR #145 adds:

- version identity;
- fresh-checkout quickstart;
- actual compatibility matrix;
- troubleshooting;
- migration guide;
- releasing guide.

PR #150 proves multi-platform package construction and checksums.

## Release decision gate

| Gate | Evidence | Status |
| --- | --- | --- |
| Foundation / typed collaboration | #125–#134 | PASS |
| Visual/local Control Plane | #138 | PASS |
| Threat model + abuse tests | #139 + #146 + THREAT_MODEL | PASS within documented scope |
| Multi-OS verification | #139, #143, #145, #146, #149, #150, #152 | PASS |
| Linux race verification | #146, #149, #150, #152 | PASS |
| Multi-agent coordinated E2E | #140 | PASS |
| Budget / quota exhaustion | #142 | PASS |
| Governed reversible memory | #143 | PASS |
| Release docs and version identity | #145 | PASS |
| Reproducible packages + checksums | #150 | PASS |
| Exact public RC tag/release | release workflow exists; tag not yet created | PENDING PUBLICATION |

## Decision

**Implementation and readiness gates for the documented `0.1.0-rc.1` scope are satisfied.**

The repository is now **ready to create the `v0.1.0-rc.1` prerelease from the exact green master line that includes #152**.

Do not describe this as:

- a stable 1.0 release;
- an independent security audit;
- universal safe execution of arbitrary untrusted test programs;
- completion of every long-term IDE/ecosystem goal.

The immediate remaining release operation is publication of the exact RC tag and verification of the published checksums/artifacts.

## After the RC

Post-RC product evolution can include:

- dedicated VS Code client/extension;
- dedicated IntelliJ client/plugin;
- broader structured Quality parsers/toolchains;
- additional external connectors/MCP integrations;
- telemetry/observability UX beyond the current local Control Plane;
- release feedback fixes before stable `v0.1.0`.

Those are future improvements, not blockers for the documented `0.1.0-rc.1` candidate.
