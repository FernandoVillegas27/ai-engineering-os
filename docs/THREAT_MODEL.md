# Threat model — AI Engineering OS

Status: release-gate threat model with executable evidence and explicit scope restrictions.
Updated: 2026-11-07.

This document is not an independent third-party security audit. It records the security boundaries required for the current release-candidate scope and the repository evidence that exercises them.

## Assets and boundaries

1. **Original repository**: source code and configuration must remain unchanged until separately approved promotion.
2. **Isolated workspaces**: agent-written content and tests are untrusted; isolation and file-scope validation are mandatory.
3. **Run event store, snapshots, archives and checkpoints**: durable evidence must resist tampering, truncation and replay confusion.
4. **Artifact/message bus and memory**: model-generated and retrieved material is untrusted data, never an instruction or permission.
5. **Runtime and tool adapters**: spawned processes, MCP tools, connectors and local utilities may execute commands or make network calls.
6. **Control Plane / CLI**: authorized human actions must be distinguished from model requests.
7. **Secrets, source files and diagnostic output**: logs and tool errors must not become an exfiltration channel.
8. **Repository-owned verification code**: test/build scripts are executable project code and are not inherently trusted just because they are tests.

## Attacker-controlled inputs

- Project repository content, including malicious test scripts, filenames, dependencies, symlinks, junctions/reparse points and prompt-injection text.
- LLM responses, copied code, suggested tool arguments and collaboration messages.
- MCP/connector responses, network content and stored memory.
- Environment variables, runtime command stdout/stderr, malformed registry or policy files.
- Persisted event/archive files altered by another local process or interrupted writes.

## Release security invariants

- No runtime is granted new permission because a tool is healthy or a model asks for it.
- No automatic promotion to the original repository.
- No pre-existing test failure, partial parse or ambiguous infrastructure failure is converted into a pass.
- No unverified event archive authorizes deletion of live history.
- No source-file or recalled-memory instructions override deterministic policy.
- No external tool can bypass a scoped task's permission envelope.
- No retry loop is unbounded.
- No mutating run executes repository-owned verification code without explicit human trust evidence.
- No deleted, superseded or expired governed memory enters automatic recall.
- Cancellation must terminate the provider/test process tree rather than only the immediate parent where supported by the OS.
- Diagnostics must redact common credential forms before surfacing external provider output.

## Executable evidence matrix

| Threat | Expected safe result | Repository evidence / boundary | Gate status |
| --- | --- | --- | --- |
| `../`, absolute, drive or UNC promotion path | reject before touching outside project | promotion relative-path validation + adversarial traversal tests | covered |
| Unix symlink escape | reject link source/parent and preserve outside content | PR #139 promotion adversarial tests | covered |
| Windows junction / reparse-point escape | reject unsafe parent identity | Windows reparse detection + junction test in R1 hardening | covered where runner can create junction |
| Original file changed after validation | reject stale preimage / TOCTOU | PR #139 changed-target tests + promotion preimage checks | covered |
| Symlink replacement with matching bytes | reject identity change even when content matches | PR #139 adversarial identity test | covered |
| Unplanned create collision | never overwrite unrelated existing target | PR #139 create-collision test | covered |
| Hostile provider proposes traversal/sensitive paths | proposal rejected before stage/apply | `core/staging/staging_test.go` traversal, Windows/backslash, sensitive-path and rename tests | covered |
| Prompt injection in repository/collaboration context | remain evidence; cannot override task/safety | Codex/Cursor prompt security tests + deterministic permissions/post-validation | covered |
| Repository-owned malicious test/build script | never run autonomously without human trust decision | `--trust-project-code` required for mutating runs; trust persisted across resume; isolated HOME/tool caches | explicitly scoped |
| Secret printed by external tool/provider | redact before doctor/health/error output | `core/toolhealth` redaction tests | covered |
| Secret stored in project memory | redact before governed ledger and optional mirror | PR #143 memory-governance tests | covered |
| Provider advertises health but auth/transport fails | classify typed failure; no permission gain/no hidden retry | PR #134 toolhealth tests | covered |
| Repeated retryable/quota failures | bounded provider attempts and terminal typed limit | PR #142 run-budget tests | covered |
| Child process survives timeout/cancel | terminate full process tree | commandmodel process-group/taskkill implementation + descendant marker E2E test | covered by multi-OS CI target |
| Archive/snapshot byte tampering | fail closed | snapshot/archive/compaction tamper tests from #122/#125/#126 | covered |
| Missing/gapped historical event | fail closed; no destructive compaction authorization | snapshot/archive/compaction sequence tests | covered |
| Interrupted compaction | resumable only after verified compacted evidence exists | #126 idempotent/resume tests | covered |
| Stale memory conflicts with current run | memory remains untrusted evidence with provenance and reversible lifecycle | #143 governed memory + prompt trust boundary | covered |
| Agent/model self-approval | promotion rejected without explicit approval | #140 promotion E2E + promotion approval gate | covered |
| Crash/pause/restart loses run state | resume from durable state/checkpoints; stale takeover explicit | runcontrol/checkpoint/recovery tests + #132 soak | covered |
| Control Plane UI mutates source of truth | UI remains read-only client of existing API/state | #138 embedded UI architecture | covered by scope |
| Browser automation turns read intent into navigation/form/network side effects | no browser action can bind without operation-scoped approval evidence; passive inspection remains separately declared | Playwright registry approval policy + toolavailability negative tests | policy covered; adapter not yet implemented |

## Repository-owned verification code: explicit release scope

Deterministic Quality may execute project commands such as:

- `go test`;
- Maven/Gradle test tasks;
- `npm test`;
- pytest.

Those commands are attacker-controlled when the repository itself is untrusted. AI Engineering OS does **not** claim a universal cross-platform filesystem/network sandbox for arbitrary third-party test programs in this release line.

Therefore the current release scope is:

1. untrusted repositories may be inspected through read-only / dry-run workflows;
2. a coordinated mutating run must include explicit `--trust-project-code`;
3. the trust decision is persisted in durable run state and required again for legacy paused runs that lack that evidence;
4. verification receives an isolated HOME/USERPROFILE/GOPATH/npm/Gradle/Maven cache environment;
5. the trust decision does not grant agent/model access to secrets, production, Git push, network tools or promotion;
6. deterministic scope/integrity/review/promotion gates remain mandatory after repository tests run.

This is an explicit autonomy exclusion, not a claim that arbitrary untrusted test programs are harmless.

## Process cancellation boundary

Provider and verification commands run through the common command runner.

On Unix-like systems, commands execute in a dedicated process group and cancellation kills the group.

On Windows, commands execute in a new process group and cancellation uses `taskkill /T /F`, with direct-process termination as a fallback.

The cross-platform process-tree test starts a descendant that would write a marker after the parent is cancelled. The marker must never appear.

## Diagnostic redaction boundary

Normalized external health output is redacted on both success and error paths.

Covered common secret forms include:

- Authorization Bearer values;
- API-key/token/secret/password assignments;
- GitHub-style personal tokens;
- OpenAI-style secret keys;
- PEM private keys.

Redaction is containment, not revocation. Any credential that may have been exposed elsewhere must still be rotated at its source.

## Promotion boundary

Promotion remains a separate human action after:

1. deterministic verification;
2. baseline-aware Quality;
3. independent review;
4. final original/staged integrity checks;
5. final scope validation;
6. contract/architecture verification;
7. persisted promotion candidate.

A model, memory item, MCP response or repository file cannot set the explicit approval bit.

## Residual risks / non-goals for this release candidate

- This is not an independent penetration test or formal verification.
- Arbitrary untrusted repository test code is outside unattended mutating scope unless explicitly trusted by the operator.
- OS and third-party runtime sandboxes may differ; Cursor native sandbox parity is not claimed on Windows.
- External provider/network services remain outside the trust boundary and can be unavailable or malicious; outputs stay untrusted.
- Playwright/browser automation is not currently runtime-bound. Navigation, interaction and browser-check execution remain unavailable until a concrete adapter supplies verified operation evidence plus explicit approval evidence.
- Local users/processes with sufficient host privileges can still tamper with the filesystem outside the application's control. Integrity checks aim to detect relevant durable-state/source changes, not replace host OS security.
- A credential already exfiltrated outside AI Engineering OS cannot be revoked by log redaction.

## Release-gate conclusion

Within the documented release-candidate scope, every critical scenario in this threat model is either:

- exercised by executable negative tests and fail-closed behavior; or
- explicitly excluded from unattended autonomy through the `--trust-project-code` boundary.

R1 may be considered satisfied only when the complete Linux/Windows/macOS CI matrix for this hardening change is green.
