# AI Engineering OS — Master Plan and Continuity Roadmap

> Canonical continuity document for the project.
>
> Purpose: preserve the full product vision, current state, execution order, architectural constraints, and acceptance criteria so development can continue safely across chats, IDE sessions, models, and contributors.

Last updated: 2026-11-06

---

## 1. Product vision

AI Engineering OS is a provider-independent engineering platform for coordinated AI-assisted software development.

The final system is not a single coding agent and not a wrapper around one model.

It is an engineering operating system where specialized roles can:

- understand a software project;
- recover durable project memory;
- classify work into capabilities;
- create an execution plan;
- delegate work to specialized agents;
- exchange structured findings, questions, decisions, and artifacts;
- use tools and skills under explicit permissions;
- implement changes in isolated workspaces;
- verify changes deterministically;
- review each other's work independently;
- retry only when evidence justifies another attempt;
- preserve useful engineering memory across sessions;
- work from Cursor, Codex, Claude Code, OpenCode, VS Code, IntelliJ, or future runtimes without changing the core contracts.

The long-term experience should feel like an engineering team, not a sequence of unrelated prompts.

Example target flow:

~~~text
User
  |
  v
Orchestrator
  |
  +--> Architect --------+
  |                      |
  +--> Researcher -------+--> shared task context / message bus
  |                      |
  +--> Backend Engineer -+
  |                      |
  +--> Frontend Engineer +
  |                      |
  +--> Database Engineer +
  |                      |
  +--> QA Engineer ------+
  |                      |
  +--> Reviewer ---------+
  |
  v
Deterministic Quality + Security Gates
  |
  v
Approved result / explicit blocker
  |
  v
Durable project memory
~~~

---

## 2. Non-negotiable architecture principles

These rules should survive every future refactor.

### 2.1 Provider independence

Core contracts must never depend on Cursor, Codex, Claude, OpenAI, Anthropic, Gentle-AI, Engram, or a specific IDE.

Providers are integrations.

### 2.2 Role independence from models

Agents are stable engineering roles.

A role must not mean a model.

Correct:

~~~text
backend-engineer
+ Java/Spring skills
+ tools
+ project context
+ runtime/model selected by policy
~~~

Incorrect:

~~~text
gpt-backend-agent
claude-reviewer
cursor-architect
~~~

### 2.3 Deterministic first, AI second

Use deterministic mechanisms whenever the decision can be made reproducibly.

Deterministic examples:

- project detection;
- registry validation;
- permission checks;
- capability coverage;
- path authorization;
- workspace integrity;
- test execution;
- failure parsing;
- comparison against baseline;
- schema validation;
- workflow state transitions.

AI examples:

- ambiguous classification;
- architecture reasoning;
- implementation;
- research;
- code review;
- remediation.

### 2.4 Fail closed

Unknown permissions, unsafe workspace state, broken integrity, unsupported file types, partial verification, infrastructure ambiguity, or malformed agent output must never silently become approval.

### 2.5 Least privilege

Agents only receive the permissions, files, tools, context, and memory required for their work unit.

### 2.6 Original repository protection

AI runtimes should not mutate the original project tree during proposal, staging, verification, correction, or review.

Promotion is a separate explicit boundary.

### 2.7 Evidence over confidence

An agent saying "this is correct" is not a quality signal.

Acceptance must depend on deterministic verification, structured evidence, independent review, or explicit user approval where required.

### 2.8 Portable memory

Durable engineering knowledge must belong to the project context, not to one chat or one model session.

### 2.9 Explicit inter-agent communication

Agents must communicate through typed messages or artifacts, not hidden shared prompt state.

### 2.10 Bounded autonomy

Loops require:

- explicit maximum attempts;
- no-progress detection;
- failure fingerprints;
- cancellation;
- cost/usage limits;
- clear terminal states.

---

## 3. Current repository architecture

Existing concepts already established in the repository:

- Agent
- Capability
- Skill
- Tool
- Project
- deterministic orchestration planner
- task classifier
- execution engine
- project detector
- controlled staging
- runtime adapters
- model policy
- local verification
- quality loop
- safe project views
- Engram memory integration boundary
- Context7 integration
- Gentle-AI integration
- runtime fallback
- structured diagnostics

Relevant documentation currently includes:

- `docs/architecture.md`
- `docs/agents.md`
- `docs/capabilities.md`
- `docs/skills.md`
- `docs/tools.md`
- `docs/projects.md`
- `docs/project-detector.md`
- `docs/task-classifier.md`
- `docs/orchestration-planner.md`
- `docs/execution-engine.md`
- `docs/controlled-stage.md`
- `docs/quality-loop.md`
- `docs/model-policy.md`
- `docs/engram.md`
- `docs/context7.md`
- `docs/gentle-ai.md`

This file does not replace those documents.

It defines the order in which they evolve into the complete product.

---

## Execution checkpoint — 2026-11-11 (pending CI)

- Baseline: master `04fd24c618fc8e1573e193f40ef0c2aa95807fec` after PR #192.
- No open PRs at milestone selection.
- Current milestone: reject ambiguous simultaneous `.ai-engineering-os/architecture.yaml` and `.yml` policies in either original or staged project. Existing behavior silently preferred YAML; the change fails closed and adds regression tests.
- Branch: `fix/architecture-ambiguous-policy-files`. Merge only after the exact head SHA passes required Ubuntu (including race), Windows, macOS and applicable packaging checks with no conflicts. Do not release or deploy.
- Next: assess atomic no-follow reads for actively mutating workspaces; existing pre-read symlink checks do not eliminate TOCTOU races.

---

## Execution checkpoint — 2026-11-11 (Go import parsing)

- Baseline master: `39bbccdc60a458328c0d528c3dc018f57f4c7b2e` (PR #193 merged); no PRs open at selection.
- Branch: `fix/architecture-go-import-parse-failclosed`.
- Security milestone: architecture checks must fail closed when a covered Go file has malformed import declarations. Previously `extractImports` ignored parser errors and returned no imports, allowing a malformed declaration to bypass the rule. Uncovered files retain existing behavior.
- Regression tests cover malformed covered and uncovered Go files.
- Merge gate: exact SHA, conflict-free, required Go tests Ubuntu/Windows/macOS, Ubuntu race, applicable packaging; no releases/deploys.
- Follow-up: evaluate platform-specific atomic no-follow reads for actively mutating workspaces; pre-read guards remain vulnerable to TOCTOU.

---

## Execution checkpoint — 2026-11-11 (root symlink guard)

- Base master `7f01c3e0288b62ad638380a795f31f6eb506f907`; no open PRs when selecting this milestone.
- Branch `fix/contractverify-reject-symlink-roots`: reject original or staged verification roots when their final component is a symlink or not a directory, before any recursive scan or contract read. Regression coverage for both roots and non-directory input.
- This guard does **not** solve symlinked ancestors or concurrent path replacement (TOCTOU). Follow-up must evaluate platform-specific atomic no-follow reads with clear portability and approval boundaries.
- Merge only on verified head SHA, conflict-free required Ubuntu/Windows/macOS CI and Ubuntu race. Do not publish releases/deploy.

---

## Execution checkpoint — 2026-11-11 (ancestor directory symlinks)

- Baseline master `9f04053422075726208982959d14a0878794086a` (PR #195 merged).
- Branch `fix/contractverify-ancestor-symlink-guard` extends root validation to the root's direct parent, in addition to the final component; deeper system ancestors are deliberately not gated because macOS uses legitimate /var symlinks. Both original and staged roots are covered.
- Regression tests cover symlinked direct parents on either root and normal nested directories.
- Scope remains pre-read only: concurrent replacement of filesystem components can still race. Platform-specific atomic no-follow traversal remains future work.
- Merge gate: exact head SHA, no conflicts, Ubuntu/Windows/macOS Go checks and Ubuntu race. No releases/deploys.

---

## Execution checkpoint — 2026-11-11 (portable ancestor-link regression)

- Baseline master `6cc1e35e75abe2cf6761d17ef831a121cc644a69` after PR #196 merged with green Ubuntu (including race), Windows, and macOS.
- Branch `test/contractverify-grandparent-symlink-portability` adds a regression ensuring legitimate macOS-style grandparent symlinks do not invalidate workspaces with real root and direct-parent directories.
- This deliberately does not broaden no-follow protections or claim TOCTOU resistance. Future atomic traversal requires platform-specific design.
- Merge gate: exact SHA, conflict-free, Ubuntu race and Ubuntu/Windows/macOS Go checks. No release/deploy.

---

## Execution checkpoint — 2026-11-11 (OpenAPI read identity hardening)

- Base master `5facecc413d539950836c63094c6ac2f556e3c44` after PR #197 merged; no open PRs at milestone selection.
- Branch `fix/contractverify-openapi-replacement-detection` adds pre/open/post file-identity checks for OpenAPI document reads. It rejects symlinked/non-regular final files and ordinary replacement detected across the read, with focused regressions.
- This is **not** atomic no-follow traversal and does not close all TOCTOU windows, especially parent directory replacement. A future cross-platform design must address those separately.
- Merge only after exact-head Ubuntu/Windows/macOS Go CI, Ubuntu race, no conflicts. No releases/deploys.

---

## Execution checkpoint — 2026-11-11 (architecture policy read identity)

- Baseline master `c213a15ed7a4c50f4d2442400273a83876d0b9b2` after PR #198 merged with required Go CI green.
- Branch `fix/contractverify-architecture-policy-stable-read` reuses the existing pre/open/post file identity guard for architecture policy YAML reads; regressions reject symlink and directory paths at the read boundary.
- Still not atomic no-follow traversal; directory replacement TOCTOU remains open for future cross-platform design.
- Merge only after exact-head Ubuntu/Windows/macOS CI and Ubuntu race, no conflicts. No release/deploy.

---

## Execution checkpoint — 2026-11-11 (architecture source read identity)

- Baseline master `66f358776eb3c5a230f930c223bf61518a6abbb8`, PR #199 merged; no open PRs at milestone selection.
- Branch `fix/contractverify-architecture-source-stable-read` reuses the pre/open/post file-identity guard for Go/Java/JS/TS architecture source reads, with regular-source and symlink regression coverage.
- Remaining limitation: not an atomic no-follow traversal; parent-directory replacement and other concurrent mutation races remain. Avoid claiming complete TOCTOU protection.
- Merge only after exact-head Ubuntu/Windows/macOS Go tests, Ubuntu race, no conflicts. No release/deploy.

---

## 4. Current checkpoint

### Completed before this roadmap update

The repository already has the foundation for:

- role-based agents;
- capability registry;
- skill registry and resolution;
- tool registry;
- project detection;
- natural-language task classification;
- deterministic agent planning;
- read-only execution;
- controlled write staging;
- Cursor and Codex runtime paths;
- model policy;
- runtime fallback;
- safe-view isolation;
- environment isolation;
- permission enforcement;
- local deterministic verification;
- independent review;
- bounded correction;
- structured Go failure parsing;
- baseline comparison primitives;
- baseline integrity foundation.

### Latest verified checkpoint — 2026-11-07

Latest merged implementation: PR #180, `feat(cli): add approved GitHub pull request prepare/create workflow`.

Latest merge:

~~~text
71aed0fde566d24127a1811773d980719af32504
~~~

Post-RC product evolution now additionally includes:

- #162: installable VS Code VSIX and IntelliJ plugin ZIP artifacts, SHA-256 evidence, combined tagged-release checksum manifest and release attachment integration;
- #163: provider auto-instruction/config isolation in sanitized runtime views;
- #164: first real AI Engineering OS-owned Context7 documentation binding;
- #166: operation-scoped external runtime binding evidence and fail-closed operation validation;
- #169: durable propagation of exact bound operations through plans, work units, run state, pause/resume, retry and runtime prompts;
- #170: AI Engineering OS-owned GitHub CLI read-only binding with independently verified repository/issue/pull-request operations and operation-scoped approval policy;
- #171: append-only Control Plane audit of bound tool authority per task, including dashboard visibility and snapshot-safe projection;
- #172: explicit project-scoped pre-run verification through `ai tools status --project <root> --verify-bindings`;
- #173: roadmap checkpoint through durable external-tool authority audit;
- #174: high-risk Playwright operation policy separating passive inspection from approval-required navigation/interaction/browser checks;
- #175: immutable human approval evidence scoped to run + task + tool + operation;
- #176: durable `ai run tool-approval request|grant|reject` lifecycle with append-only audit/evidence;
- #177: Control Plane projection/dashboard audit of exact tool-operation approval state;
- #178: request-bound approvals using an exact SHA-256 digest of the concrete side-effect request;
- #179: first trusted GitHub mutation adapter, limited to request-approved `create-pull-request`;
- #180: usable `ai run github-pr prepare|create` operator workflow with pre-mutation audit and exact request reconstruction.

The documented `0.1.0-rc.1` implementation/readiness scope remains complete.

The exact public tag/release publication remains a separate operation. The repository contains the exact-tag-only release workflow and builds:

- CLI linux/amd64;
- CLI windows/amd64;
- CLI darwin/amd64;
- CLI darwin/arm64;
- VS Code VSIX;
- IntelliJ plugin ZIP;

with individual SHA-256 sidecars and one combined release manifest.

The public `v0.1.0-rc.1` tag is still not present at this checkpoint.

### Current post-RC product level

Implemented beyond the original roadmap:

- baseline-aware structured Quality for Go, Java/Maven/Gradle, Jest/Vitest, pytest and supported Angular unit-test builders;
- deterministic OpenAPI/architecture gates;
- long-run recovery/compaction soak evidence;
- governed project memory/team learning;
- normalized external tool/MCP health;
- embedded visual Control Plane;
- dedicated, installable VS Code and IntelliJ clients;
- privacy-preserving local observability;
- strict required-tool planning;
- provider auto-instruction isolation;
- operation-scoped runtime availability and binding evidence;
- durable exact tool-operation authority through execution/recovery;
- AI-OS-owned Context7 documentation binding;
- AI-OS-owned GitHub read-only binding;
- operation-scoped GitHub approval boundaries that keep mutation/review operations unavailable without explicit approval evidence;
- immutable human approval evidence scoped to run + task + tool + operation, with optional expiry;
- request-bound approval digests that prevent parameter substitution after human approval;
- first request-approved GitHub mutation path for pull-request creation only;
- Control Plane audit of exact task tool authority and exact pending/granted/rejected tool-operation approvals;
- explicit project-scoped binding verification before execution;
- multi-OS and Linux race verification;
- reproducible CLI + IDE release packaging.

### External-tool authority boundary now on master

External capability is no longer represented by a coarse healthy/unhealthy tool flag.

The enforced chain is:

~~~text
Tool Registry declaration
  -> provider health / availability evidence
  -> exact bound operations
  -> operation approval policy
  -> orchestration narrowing
  -> WorkUnit.ToolOperations
  -> durable run tool_operations
  -> resume / mutation-resume / targeted retry
  -> prompt-visible bound operation summary
  -> append-only task.created audit
  -> Control Plane projection / dashboard
~~~

Rules:

1. health never grants permissions;
2. binding can only narrow operations declared in the Tool Registry;
3. approval-required operations cannot become runtime-bound without approval evidence;
4. Cursor/Codex see only the exact bound operation names, not a provider's theoretical operation set;
5. external evidence remains untrusted data and cannot change task, permissions, model policy or promotion authority;
6. older durable runs/events without operation metadata remain readable and never gain inferred authority;
7. Control Plane state is an audit projection, not a second permission source.

### Verified adapters

#### Context7

Current AI-OS-owned operation:

~~~text
search-docs
~~~

The adapter performs bounded public-library documentation retrieval only when deterministic project detection yields public library hints.

#### GitHub

Current AI-OS-owned operations:

~~~text
read-repository
read-issues
read-pull-requests
~~~

The trusted adapter uses fixed non-mutating `gh` command shapes, controlled environment forwarding and bounded JSON evidence.

GitHub read operations remain independently verified as before.

The first mutation operation is now implemented:

~~~text
create-pull-request
~~~

It is not generally runtime-bound to models. It is available only through the AI Engineering OS-owned operator workflow:

~~~text
github-pr prepare
  -> exact request digest
  -> human tool-approval grant
  -> immutable approval evidence
  -> github-pr create
  -> fixed gh pr create adapter
~~~

The approved digest covers repository, head, base, title and body. Any changed mutation-relevant field requires a new approval. Branch creation, commit, push, review, merge, edit, close and delete remain unavailable.

### Explicit operator verification

The conservative command:

~~~text
ai tools status
~~~

does not perform project-scoped external reads.

Explicit read-only verification is:

~~~text
ai tools status --project <project-root> --verify-bindings
~~~

This verifies Context7/GitHub bindings using the same operation/approval invariants as real runs.

### Immediate next milestone

**Continue external side-effect adapters only where a concrete trusted protocol/runtime and exact request boundary can be verified.**

The approval foundation and first GitHub write path are now complete enough to reuse.

Next candidate work, in safe order:

1. harden/reuse the request-bound approval primitive for any additional GitHub mutation selected as genuinely necessary;
2. define a concrete trusted Playwright/browser runtime contract before binding any browser operation;
3. add Playwright passive/read behavior first if the chosen protocol can prove that it does not silently navigate/interact;
4. add navigation/interactions/browser checks only behind exact request-bound human approval;
5. keep Codegraph unimplemented until a real provider interface is selected and verified.

Do not invent a Codegraph, Playwright, or other provider protocol merely because the Tool Registry contains a logical tool entry.

For every future adapter require:

1. a real provider/CLI/API contract that can be tested;
2. fixed or strictly bounded command/request shapes;
3. controlled environment/credential forwarding;
4. exact operation-scoped binding;
5. explicit approval boundaries for mutation-capable operations;
6. bounded untrusted evidence;
7. durable propagation/audit through the existing authority chain;
8. fail-closed behavior when provider identity, health, transport or operation evidence is ambiguous.

Known remaining product evolution toward the broadest vision:

- a real Codegraph adapter only after a concrete provider interface is selected and verified;
- a carefully scoped Playwright/browser adapter with side-effect/approval policy before any browser operation becomes bound;
- richer IDE-native views/actions without moving coordination authority into the IDE;
- broader language/toolchain Quality coverage only where real projects require it;
- stable-release feedback fixes and compatibility hardening;
- public RC/stable tag/release operations.

Do not reintroduce role-specific model pinning or weaken promotion, integrity, approvals, recovery, budgets, memory governance, runtime-instruction isolation, operation-scoped binding, credential isolation, or permission boundaries.

---

# 5. Delivery roadmap

The roadmap is intentionally incremental.

Every phase must leave the repository in a usable, tested, provider-independent state.

---

## Phase P2b — Lazy baseline integration in Quality Loop

### Goal

Use the P2a baseline provider from the production Quality Loop so pre-existing project failures do not waste correction/model quota.

### Required behavior

On the first failed deterministic verification that is not an infrastructure failure:

1. lazily request a baseline;
2. verify baseline integrity;
3. compare current staged failures against baseline failures;
4. classify:
   - pre-existing;
   - regression;
   - changed cause;
   - resolved;
   - unclassified;
   - blocking;
5. tolerate only safe pre-existing failures;
6. never tolerate compile/build blockers;
7. never tolerate partial parsing;
8. never tolerate infrastructure failures;
9. never tolerate integrity failures;
10. never send the Corrector failures that are provably only pre-existing.

### Corrector input

The Corrector should receive only actionable current failures:

- regressions;
- changed causes;
- unclassified failures when policy requires correction;
- review findings.

### No-progress protection

Fingerprint actionable failures.

If the same actionable failure set repeats after a correction attempt:

- stop correction;
- return explicit no-progress outcome;
- do not keep spending model quota.

### Review behavior

If verification fails only because of tolerated pre-existing test failures:

- run one independent review;
- never mark `Passed=true`;
- report a non-passing-but-no-new-regression outcome explicitly.

### CLI wiring

Wire the provider in `cmd/ai/stage.go` using:

- `StageResult.PreImage`;
- original project root;
- staged workspace;
- declared/authorized file scope;
- same verifier implementation.

### Acceptance criteria

- full unit coverage for comparison branches;
- integration test using real `go test`;
- repeated failure fingerprint stops correction;
- baseline is never requested on clean verification;
- baseline is never requested for pure infrastructure failure;
- `IntegrityError` never reaches Corrector;
- compile blockers remain blocking regardless of baseline;
- original tree remains unchanged.

---

## Phase P2c — Quality outcome model and diagnostics

### Goal

Make Quality Loop outcomes machine-readable and usable by CLI, future agents, telemetry, and orchestration.

### Add explicit terminal reasons

Examples:

- `passed`
- `preexisting-only`
- `verification-regression`
- `review-rejected`
- `integrity-failure`
- `infrastructure-failure`
- `partial-verification`
- `no-progress`
- `max-attempts`
- `cancelled`

### Outcome should include

- verification status;
- baseline status;
- comparison summary;
- blocking failures;
- correction attempts;
- review attempts;
- final fingerprints;
- runtime/provider used;
- whether model escalation occurred;
- user-safe diagnostics.

### Security

Do not expose:

- file contents;
- secrets;
- absolute private paths;
- hashes that can leak protected material;
- raw provider payloads containing sensitive context.

---

## Phase P3 — Promotion boundary

### Goal

Introduce the first safe path from an approved staged workspace back into the original project.

### Rules

Promotion must:

- be explicit;
- verify the original tree still matches the expected pre-image;
- revalidate staged authorized changes;
- present a deterministic change summary;
- refuse undeclared files;
- refuse sensitive/unsupported paths;
- refuse if original repository changed;
- support dry run;
- preserve rollback information;
- never automatically push.

### Initial commands

Potential shape:

~~~text
ai stage ...
ai inspect-stage ...
ai apply <stage-id>
~~~

### Approval

At first, promotion should require explicit user approval.

Future policies may allow constrained auto-apply, but not until auditability is mature.

---

## Phase P4 — Durable run/session model

### Goal

Make every engineering flow resumable and inspectable.

### Introduce IDs

- `run_id`
- `task_id`
- `work_unit_id`
- `agent_run_id`
- `stage_id`
- `verification_id`
- `review_id`

### Persist structured state

A run record should preserve:

- original task;
- project identity;
- detected stack;
- requested capabilities;
- selected agents;
- skills;
- tools;
- permission envelope;
- runtime/model policy;
- work-unit dependencies;
- artifacts;
- messages;
- verification results;
- approvals;
- final state.

### Required property

A new process must be able to inspect an old run without needing the original chat transcript.

---

## Phase P5 — Project memory v1

### Goal

Turn Engram from an integration boundary into real project continuity.

### Memory categories

Persist curated engineering memory such as:

- architecture decisions;
- project conventions;
- known constraints;
- environment quirks;
- accepted trade-offs;
- recurring failure patterns;
- important commands;
- integration knowledge;
- unresolved blockers;
- completed milestones.

Do not store every prompt or full transcript by default.

### Memory lifecycle

~~~text
run starts
  -> retrieve project memory
  -> rank relevant memories
  -> inject bounded context
  -> execute work
  -> propose memory candidates
  -> validate/redact
  -> persist durable memories
  -> write session summary
~~~

### Memory safety

- project namespace isolation;
- no secrets;
- source attribution;
- timestamps;
- confidence/status;
- supersession/deprecation;
- bounded retrieval;
- user-visible deletion path;
- deterministic project identity.

### Required future abstraction

Core memory interfaces should support providers other than Engram.

---

## Phase P6 — Artifact protocol

### Goal

Give agents a provider-independent way to exchange engineering work products.

### Artifact types

Initial structured artifacts:

- `TaskBrief`
- `ArchitectureDecision`
- `ResearchNote`
- `ImplementationPlan`
- `ChangeProposal`
- `CodePatchReference`
- `TestPlan`
- `VerificationReport`
- `ReviewReport`
- `RiskAssessment`
- `Question`
- `Decision`
- `Handoff`

### Every artifact should contain

- stable ID;
- producer agent;
- run/task ID;
- type;
- schema version;
- created timestamp;
- related artifacts;
- evidence references;
- status;
- human-readable summary;
- structured payload.

### Key rule

Agents exchange artifact references and structured summaries.

They should not need to replay entire provider conversations.

---

## Phase P7 — Inter-agent message bus

### Goal

Allow agents to talk to one another explicitly.

This is the phase that introduces real agent-to-agent communication.

### Message envelope

A provider-independent message contract should include:

~~~yaml
id:
run_id:
task_id:
from_agent:
to:
kind:
priority:
reply_to:
artifact_refs:
payload:
created_at:
expires_at:
~~~

### Initial message kinds

- `request`
- `response`
- `question`
- `answer`
- `finding`
- `proposal`
- `challenge`
- `decision`
- `handoff`
- `blocker`
- `status`

### Addressing

Support:

- one agent;
- role group;
- orchestrator;
- broadcast only when explicitly allowed.

### Safety

Messages do not grant permissions.

Receiving a request never expands the target agent's permission envelope.

### Persistence

Messages should be persisted with the run state for replay and debugging.

### Ordering

Use deterministic per-run sequencing.

Do not rely only on provider timestamps.

### Loop prevention

Prevent infinite agent conversations with:

- hop limits;
- per-task message limits;
- deduplication;
- repeated-question fingerprints;
- deadlines/cancellation;
- orchestrator intervention.

---

## Phase P8 — Dependency-aware multi-agent scheduler

### Goal

Execute multiple specialized work units as a coordinated team.

### Move from

~~~text
one selected worker -> one execution
~~~

to:

~~~text
task graph
  -> workers
  -> dependencies
  -> parallel safe execution
  -> aggregation
~~~

### Work graph

Represent tasks as a DAG.

Example:

~~~text
Architecture
   |
   +------> Backend implementation ----+
   |                                   |
   +------> Frontend implementation ---+--> QA --> Reviewer
   |                                   |
   +------> Database design -----------+
~~~

### Scheduler responsibilities

- dependency resolution;
- ready queue;
- concurrency limits;
- resource locks;
- file-scope conflict detection;
- cancellation propagation;
- retries;
- terminal failure propagation.

### Parallelism rule

Two mutating agents must not concurrently modify overlapping authorized paths unless a future merge protocol explicitly supports it.

---

## Phase P9 — Orchestrator v2

### Goal

Turn the Orchestrator into a real coordinator instead of only deterministic routing infrastructure.

### Responsibilities

The Orchestrator should:

- understand the high-level user objective;
- request capability classification;
- build/adjust a task graph;
- delegate to agents;
- collect questions;
- resolve dependencies;
- request architecture decisions;
- escalate blockers;
- track budget;
- decide when more work is unnecessary;
- synthesize final status.

### What it must not do

The Orchestrator must not:

- silently bypass permission rules;
- directly mark failed verification as passed;
- rewrite independent review evidence;
- hide blockers;
- invent tool availability;
- grant itself production/secrets access.

### Hybrid design

Use AI for ambiguous decomposition.

Use deterministic validation for the generated plan.

---

## Phase P9.5 — Multi-agent Control Plane

### Goal

Expose and govern the live multi-agent system as an inspectable control plane.

This is a functional requirement, not a visual-dashboard requirement.

The system must be able to answer, in real time or from persisted run state:

- which agents exist in the current run;
- which agents are active, waiting, blocked, completed, failed, cancelled, or retrying;
- which task/work unit each agent owns;
- which agent is waiting on another agent;
- what dependencies exist between work units;
- what messages agents sent to each other;
- what artifacts each agent produced or consumed;
- what file scope each mutating agent owns;
- what runtime/model/provider each invocation used;
- what verification/review gates have passed or failed;
- what retry/correction budget remains;
- what memory/context was loaded;
- what tool calls were attempted and whether they succeeded;
- what approvals are currently required;
- why the orchestrator made a routing, retry, escalation, or stop decision;
- aggregate progress for the whole run.

### Functional model

The control plane should be driven from authoritative run events and persisted state rather than scraping provider conversations.

~~~text
Agents / Runtimes
      |
      v
Message Bus + Artifact Store
      |
      v
Run Event Stream
      |
      +--> Task Graph / Scheduler State
      +--> Agent State Registry
      +--> Quality / Verification State
      +--> Tool / Runtime State
      +--> Memory / Context State
      +--> Budget / Retry State
      +--> Approval State
      |
      v
Control Plane API / CLI / future UI
~~~

### Agent state machine

At minimum, support explicit states such as:

- `queued`;
- `ready`;
- `running`;
- `waiting`;
- `blocked`;
- `verifying`;
- `reviewing`;
- `correcting`;
- `completed`;
- `failed`;
- `cancelled`.

Transitions must be explicit and auditable.

### Run event stream

Every important transition should emit a typed event.

Examples:

- `run.started`;
- `task.created`;
- `task.assigned`;
- `agent.started`;
- `agent.message.sent`;
- `artifact.created`;
- `agent.waiting`;
- `agent.blocked`;
- `verification.started`;
- `verification.failed`;
- `baseline.compared`;
- `correction.started`;
- `review.rejected`;
- `approval.requested`;
- `agent.completed`;
- `run.completed`.

The control plane should be reconstructable by replaying persisted events plus snapshots/checkpoints.

### Orchestrator visibility

The orchestrator must have a deterministic view of:

- active task graph;
- agent states;
- open questions;
- blockers;
- dependency readiness;
- owned file scopes;
- verification state;
- retry counts;
- budgets;
- pending approvals.

This same state should also be inspectable by a human.

### Human control

The control plane should eventually support commands such as:

~~~text
ai run status <run-id>
ai run agents <run-id>
ai run graph <run-id>
ai run messages <run-id>
ai run artifacts <run-id>
ai run pause <run-id>
ai run resume <run-id>
ai run cancel <run-id>
ai run retry <run-id> <task-id>
~~~

Human intervention must be explicit and recorded as events.

### Progress

Progress must be derived from task graph state, not invented percentages from an LLM.

Useful metrics include:

- total tasks;
- ready/running/waiting/blocked/completed tasks;
- completed required dependencies;
- current critical path;
- corrections/retries used;
- verification gates completed;
- messages pending response.

### Important boundary

A future 2D/3D interface may visualize this state, but the backend control-plane semantics must exist first.

The visual layer must remain replaceable.

The product goal is therefore:

> Build the multi-agent coordination engine and observable control plane first; any dashboard is only one client of that system.

---

## Phase P10 — Specialist collaboration patterns

### Goal

Define repeatable collaboration protocols between roles.

### Architect -> Implementers

Architect produces:

- architecture decision;
- constraints;
- interfaces;
- file/module boundaries;
- explicit unknowns.

Implementers may challenge the plan through a structured `challenge` message.

### Backend <-> Frontend

Exchange versioned API contracts rather than prose-only assumptions.

Potential artifact:

- `APIContract`

### Backend <-> Database

Exchange:

- schema requirements;
- transaction rules;
- migration constraints;
- query expectations.

### Implementer -> QA

Provide:

- changed behavior;
- risks;
- intended invariants;
- suggested test surfaces.

QA independently decides what to verify.

### QA -> Implementer

QA sends reproducible failures with structured evidence.

### Reviewer -> Orchestrator

Reviewer remains independent and reports:

- severity;
- evidence;
- blocking status;
- required correction.

Reviewer should not directly rewrite its own findings into approval.

### Researcher -> Team

Research outputs must include:

- source/evidence references;
- freshness;
- uncertainty;
- recommendation vs fact distinction.

---

## Phase P11 — Shared blackboard / project state

### Goal

Provide agents a bounded shared state that is safer than unconstrained shared conversation context.

### Blackboard contents

Examples:

- current objective;
- accepted architecture decisions;
- active task graph;
- open questions;
- blockers;
- API contracts;
- changed file scopes;
- verification status;
- artifacts;
- budget state.

### Rules

- schema validated;
- versioned;
- append/event based where practical;
- no raw secrets;
- agents read only the sections needed by their work unit;
- write permissions are scoped by artifact/message type.

---

## Phase P12 — Merge and conflict coordination

### Goal

Support multiple mutating specialist agents in one run.

### Initial strategy

Prefer file-scope partitioning.

Example:

~~~text
backend-engineer
  -> backend/**

frontend-engineer
  -> frontend/**

database-engineer
  -> migrations/**
~~~

### Later strategy

Support isolated branches/worktrees or stage trees per work unit and deterministic integration.

### Conflict handling

Never ask an LLM to blindly resolve a source conflict without context.

Conflict resolution should receive:

- base;
- both proposed versions;
- task ownership;
- artifacts/decisions;
- tests.

Then re-run full verification and review.

---

## Phase P13 — Quality Loop v2 for multi-agent runs

### Goal

Quality validation becomes run-wide instead of only stage-local.

### Layers

1. worker-local checks;
2. integration checks;
3. baseline comparison;
4. contract checks;
5. architecture constraint checks;
6. security checks;
7. independent review;
8. optional specialist reviews.

### Examples of specialist review

- security reviewer;
- accessibility review;
- database migration review;
- API compatibility review;
- frontend visual review.

### Required invariant

No agent can approve its own mutating work as the only quality gate.

---

## Phase P14 — Cost, quota, and intelligence routing

### Goal

Use stronger models only when justified.

### Budget model

Track at least:

- runtime invocations;
- model invocations;
- retries;
- correction attempts;
- review attempts;
- elapsed execution time;
- provider quota failures.

### Routing signals

Escalation may depend on:

- task complexity;
- repeated rejection;
- architecture ambiguity;
- diff size;
- security criticality;
- failure history;
- user-selected policy.

### Default philosophy

Economical first, escalate on evidence.

Do not hard-code one model per role.

---

## Phase P15 — Tool execution framework

### Goal

Let agents use tools through platform contracts rather than runtime-specific prompt tricks.

### Tool contract needs

- ID;
- capabilities;
- input schema;
- output schema;
- side-effect classification;
- permissions;
- health;
- timeout;
- audit information.

### Side-effect classes

Potential classes:

- `read-only`
- `workspace-write`
- `repository-write`
- `external-write`
- `production`

### Approval

Tool use above the work unit's granted level must stop and request explicit approval.

---

## Phase P16 — MCP and external capability mesh

### Goal

Make external integrations discoverable and usable by agents.

Potential integrations:

- Context7;
- Engram;
- Gentle-AI;
- GitHub;
- Jira;
- Figma;
- browser/devtools;
- databases;
- CI systems;
- documentation systems.

### Important distinction

A tool being registered does not mean:

- it is installed;
- it is authenticated;
- it is healthy;
- the current agent has permission to use it.

Those are separate states.

---

## Phase P17 — IDE/runtime adapters

### Goal

Expose the same project/team behavior from different development environments.

Target adapters:

- Cursor;
- Codex CLI;
- Claude Code;
- OpenCode;
- VS Code;
- IntelliJ IDEA;
- future providers.

### Core requirement

Switching runtime must not change:

- project identity;
- agent IDs;
- capability semantics;
- skill semantics;
- artifact schemas;
- memory semantics;
- quality rules;
- permission rules.

---

## Phase P18 — Developer UX

### Goal

Make the platform practical for daily use.

Potential commands:

~~~text
ai doctor
ai inspect
ai plan
ai execute
ai propose
ai stage
ai quality
ai apply
ai run
ai resume
ai status
ai messages
ai artifacts
ai memory
ai agents
ai skills
ai tools
~~~

### Human-readable run view

A developer should be able to answer:

- What are the agents doing?
- Why was this agent selected?
- What changed?
- What failed?
- What is pre-existing?
- Who is waiting for whom?
- How much retry budget remains?
- What requires my approval?
- What memory was used?
- What memory will be saved?

---

## Phase P19 — Observability and audit trail

### Goal

Every important engineering decision becomes explainable after the run.

### Events

Examples:

- task classified;
- agent selected;
- skill attached;
- tool resolved;
- runtime selected;
- model escalated;
- message sent;
- artifact produced;
- stage created;
- verification failed;
- baseline requested;
- regression detected;
- correction requested;
- review rejected;
- approval requested;
- promotion completed.

### Logging rules

Never log secrets or raw private source contents by default.

Support correlation by:

- run ID;
- task ID;
- work unit ID;
- agent run ID.

---

## Phase P20 — Policy engine

### Goal

Move critical behavior out of ad-hoc CLI flags into explicit, testable policy.

Policy domains may include:

- model policy;
- runtime policy;
- retry policy;
- quality policy;
- approval policy;
- tool policy;
- memory policy;
- concurrency policy;
- promotion policy.

### Example

~~~yaml
quality:
  compile_errors: block
  partial_parse: block
  preexisting_test_failures: tolerate_without_pass
  max_corrections: 3

promotion:
  require_user_approval: true
  require_original_integrity: true
~~~

Policies must not be allowed to weaken hard security invariants.

---

## Phase P21 — Security hardening

### Goal

Treat the platform as an execution system, not only as a prompt framework.

Required areas:

- path traversal defenses;
- symlink/junction/reparse-point defenses;
- TOCTOU reduction;
- process environment isolation;
- credential exclusion;
- subprocess timeouts;
- output size limits;
- workspace cleanup;
- untrusted repository content;
- prompt injection in source files;
- malicious test scripts;
- tool injection;
- MCP trust boundaries;
- archive handling;
- binary files;
- network policy;
- supply-chain risk;
- audit redaction.

### Threat model

Maintain a formal threat-model document before high-autonomy execution is enabled.

---

## Phase P22 — Cross-language quality providers

### Goal

Bring structured verification parity beyond Go.

Add structured adapters for:

- Java/Maven;
- Java/Gradle;
- Node/Jest/Vitest/Angular;
- Python/Pytest;
- later .NET and others.

Each adapter should prefer machine-readable output when reliable.

### Required normalized failure identity

Avoid relying on workspace-specific absolute paths.

Failure signatures should be stable enough for baseline comparison.

---

## Phase P23 — Contract and architecture verification

### Goal

Quality should validate more than tests.

Potential deterministic checks:

- OpenAPI compatibility;
- schema compatibility;
- migration safety;
- dependency rules;
- forbidden imports;
- architectural boundaries;
- linting;
- formatting;
- generated-file consistency.

---

## Phase P24 — Long-running autonomous runs

### Goal

Support larger features composed of multiple tasks while remaining bounded and interruptible.

### Required first

Do not enable this phase until:

- durable run state exists;
- message bus exists;
- artifact protocol exists;
- multi-agent scheduler exists;
- no-progress detection exists;
- cost limits exist;
- audit trail exists;
- explicit approvals exist;
- recovery/resume exists.

### Features

- pause;
- resume;
- cancel;
- retry one task;
- replace one provider;
- replan remaining work;
- inspect live blockers.

---

## Phase P25 — Team learning

### Goal

Use historical project evidence to improve future execution without turning memory into uncontrolled self-modifying policy.

Possible learning:

- known test commands;
- preferred architecture conventions;
- recurrent project-specific pitfalls;
- reliable specialists for certain task classes;
- stable runtime compatibility information;
- past successful plans.

### Guardrail

Learned data may influence ranking/context.

It must not silently modify hard permissions or security policy.

---

## Phase P26 — Production-grade release

### Goal

Reach a stable, documented, testable version suitable for real multi-agent engineering workflows.

### Release requirements

- stable schemas;
- schema migrations/versioning;
- full CLI help;
- architecture docs;
- threat model;
- recovery docs;
- Windows/Linux support;
- CI matrix;
- deterministic test suite;
- integration test suite;
- runtime contract tests;
- provider fallback tests;
- memory tests;
- message bus tests;
- scheduler tests;
- quality tests;
- audit tests;
- sample projects;
- onboarding guide.

---

# 6. Target multi-agent architecture

The complete architecture should converge toward this shape:

~~~text
                         +----------------------+
                         |       User / IDE     |
                         +----------+-----------+
                                    |
                                    v
                         +----------------------+
                         |     Orchestrator     |
                         +----------+-----------+
                                    |
                    +---------------+----------------+
                    |                                |
                    v                                v
          +-------------------+          +----------------------+
          | Task Classifier   |          | Project Context     |
          | + Capability Map  |          | + Durable Memory    |
          +---------+---------+          +----------+-----------+
                    |                               |
                    +---------------+---------------+
                                    |
                                    v
                         +----------------------+
                         | Deterministic Planner|
                         +----------+-----------+
                                    |
                                    v
                         +----------------------+
                         | Task Graph Scheduler |
                         +----------+-----------+
                                    |
             +----------------------+----------------------+
             |         |          |          |             |
             v         v          v          v             v
        Architect   Backend    Frontend      QA         Researcher
             |         |          |          |             |
             +---------+----------+----------+-------------+
                                    |
                                    v
                         +----------------------+
                         | Message / Artifact   |
                         | Bus + Blackboard     |
                         +----------+-----------+
                                    |
                                    v
                         +----------------------+
                         | Isolated Workspaces  |
                         +----------+-----------+
                                    |
                                    v
                         +----------------------+
                         | Verification         |
                         | Baseline Comparison  |
                         | Independent Review   |
                         +----------+-----------+
                                    |
                                    v
                         +----------------------+
                         | Promotion / Approval |
                         +----------+-----------+
                                    |
                                    v
                         +----------------------+
                         | Memory + Audit       |
                         +----------------------+
~~~

---

# 7. Agent roles in the complete version

The initial stable roles remain:

1. `orchestrator`
2. `architect`
3. `researcher`
4. `frontend-engineer`
5. `backend-engineer`
6. `database-engineer`
7. `qa-engineer`
8. `reviewer`

Additional roles should only be added when a clear reusable responsibility cannot be represented as a skill/capability.

Possible later roles:

- security-engineer;
- devops-engineer;
- accessibility-reviewer;
- performance-engineer.

Do not create framework agents such as `angular-agent` or `spring-agent`.

Those remain skills attached to stable roles.

---

# 8. Agent communication rules

The final product must make agent collaboration explicit and auditable.

## Agents may

- ask another agent a scoped question;
- request an artifact;
- challenge a decision;
- report a blocker;
- hand off a task;
- publish a finding;
- respond with evidence;
- request orchestrator intervention.

## Agents may not

- grant permissions to another agent;
- change another agent's role contract;
- hide quality failures;
- approve their own work as the only reviewer;
- communicate secrets through messages;
- create infinite discussion loops;
- bypass the deterministic task graph.

## Example

~~~text
backend-engineer
  -> Question(API payload ambiguity)
  -> architect

architect
  -> Decision(versioned API contract)
  -> backend-engineer
  -> frontend-engineer

backend-engineer
  -> APIContract artifact
  -> frontend-engineer
  -> qa-engineer

qa-engineer
  -> VerificationReport(failure)
  -> backend-engineer

backend-engineer
  -> corrected staged artifact

reviewer
  -> ReviewReport(approved/rejected)
  -> orchestrator
~~~

---

# 9. Memory strategy

Memory exists at multiple levels.

## Project memory

Long-lived project-specific knowledge.

## Run state

Exact state required to resume an active or completed engineering run.

## Artifacts

Versioned engineering outputs created by agents.

## Messages

Coordination history for one run.

## Provider conversation context

Temporary and disposable.

The architecture must assume provider conversation context can disappear at any time.

If losing a chat destroys project continuity, the architecture is incomplete.

---

# 10. Definition of "agents talking to each other"

This phrase has a precise meaning in the target product.

It does **not** mean:

- placing multiple persona prompts in one LLM conversation;
- letting one model pretend to be several agents;
- giving every agent the entire shared chat history.

It means:

1. each agent invocation has a stable role identity;
2. each invocation has a scoped work unit;
3. agents produce structured artifacts;
4. agents send typed messages;
5. messages/artifacts are persisted;
6. the orchestrator and scheduler control routing;
7. permissions remain independent;
8. agents may run through different runtime/model providers;
9. the run can be reconstructed without hidden provider state.

---

# 11. Quality invariants for the final system

The following must always remain true:

- compile/build failures block;
- integrity failures block;
- infrastructure failures do not become code regressions;
- partial verification never implies success;
- pre-existing failures may be tolerated but never converted into a false pass;
- new regressions are actionable;
- repeated identical failures stop bounded correction;
- independent review cannot be replaced by implementer self-confidence;
- unauthorized file modifications block;
- sensitive file access blocks;
- original repository mutations are guarded;
- promotion requires integrity validation;
- every autonomous loop is bounded.

---

# 12. Security invariants

Hard invariants should not be configurable away.

- secrets denied by default;
- production denied by default;
- unknown permissions rejected;
- unknown workspace file types rejected unless deliberately authorized;
- symlinks/special files rejected at sensitive execution boundaries;
- subprocess environment allowlisted;
- runtime workspaces sanitized;
- absolute private paths minimized in model-visible diagnostics;
- provider output treated as untrusted input;
- agent messages treated as untrusted input;
- repository text may contain prompt injection and must never override platform policy.

---

# 13. Testing strategy

Every future phase should include the lowest appropriate layers.

## Unit tests

For:

- contracts;
- parsing;
- policy;
- comparison;
- state machines;
- message validation;
- artifact validation;
- scheduler logic.

## Integration tests

For:

- real subprocesses;
- real local verification;
- baseline comparison;
- staging;
- workspace promotion;
- runtime adapters;
- Engram adapter;
- message persistence.

## Cross-platform tests

At minimum:

- Windows;
- Linux.

## Mutation tests / invariant removal tests

For security-critical invariants, temporarily remove or weaken one invariant and confirm tests fail.

This technique was already useful in P2a and should continue.

## End-to-end fixtures

Maintain small representative fixture projects for:

- Go;
- Java/Maven;
- Java/Gradle;
- Node;
- Python.

---

# 14. Branch and PR discipline

Recommended workflow:

~~~text
master
  -> feature branch
  -> focused PR
  -> CI
  -> review/check
  -> merge
~~~

Rules:

- one architectural milestone per PR where practical;
- no unrelated cleanup mixed into safety-critical PRs;
- no force push to `master`;
- avoid hidden behavior changes;
- PR body must include:
  - context;
  - architecture;
  - changes;
  - security;
  - tests;
  - limitations;
  - next step;
  - continuity note.

---

# 15. Recommended immediate PR sequence

Continue from the current checkpoint with small reviewable PRs.

### PR P2b-1

Quality Loop lazy baseline integration.

### PR P2b-2

No-progress fingerprints and explicit baseline-aware outcome.

Could be combined with P2b-1 if the implementation remains clear and well tested.

### PR P2c

Structured Quality Outcome + CLI diagnostics.

### PR P3a

Promotion design/contracts + dry run.

### PR P3b

Safe apply to original project with integrity checks.

### PR P4

Durable run/session model.

### PR P5

Real Engram memory read/write lifecycle.

### PR P6

Artifact schemas and storage.

### PR P7

Inter-agent message contract + persistence.

### PR P8

Task DAG + scheduler.

### PR P9

Orchestrator v2 using task graph and message/artifact system.

After P9, the repository begins to behave like the intended multi-agent engineering team.

---

# 16. What not to do yet

Do not prematurely:

- add dozens of agent roles;
- let agents directly edit `master`;
- let agents push automatically;
- allow production access;
- allow secret access;
- add unbounded autonomous loops;
- build a distributed queue before the local run model exists;
- hard-code models to roles;
- let AI choose permissions;
- use chat transcript as the primary persistence layer;
- implement multi-agent parallel writes before file conflict coordination exists;
- optimize away deterministic safety checks for speed.

---

# 17. Completion levels

## Level 1 — Safe single-agent engineering

Already substantially built.

- classify;
- plan;
- stage;
- verify;
- review;
- correct;
- protect original.

## Level 2 — Baseline-aware engineering

P2a + P2b/P2c.

- distinguish pre-existing failures;
- avoid useless corrector calls;
- no-progress stopping;
- explicit quality diagnostics.

## Level 3 — Resumable engineering

P3-P5.

- safe promotion;
- durable runs;
- project memory.

## Level 4 — Collaborative agents

P6-P10.

- artifacts;
- message bus;
- scheduler;
- Orchestrator v2;
- specialist collaboration.

This is the first level where agents truly "talk to each other."

## Level 5 — Multi-agent engineering OS

P11-P20.

- blackboard;
- conflict coordination;
- multi-agent quality;
- budgets;
- tools;
- MCP mesh;
- IDE adapters;
- audit;
- policies.

## Level 6 — Production autonomy

P21-P26.

- hardened threat model;
- cross-language structured quality;
- architecture/contract gates;
- long-running bounded autonomy;
- team learning;
- production-grade release.

---

# 18. Success criteria for the complete version

The project reaches the intended full version when a user can give a high-level task such as:

> Add a new business feature that requires backend, frontend, database changes and tests.

And AI Engineering OS can safely:

1. identify the project;
2. restore relevant memory;
3. classify the task;
4. create a validated task graph;
5. ask the Architect for constraints when needed;
6. delegate backend/frontend/database work;
7. let agents exchange contracts and questions;
8. execute work in isolated scopes;
9. integrate the results;
10. run deterministic verification;
11. compare failures against the original baseline;
12. ask QA to investigate regressions;
13. let implementers correct actionable failures;
14. stop if the same failure repeats;
15. run independent review;
16. report blockers transparently;
17. request user approval for sensitive boundaries;
18. safely promote approved changes;
19. persist decisions and useful memory;
20. allow a later session in another IDE/provider to resume from the same project state.

No single model conversation should be required to remember the entire journey.

---

# 19. Resume-here instructions for future chats

When starting a new ChatGPT/Cursor/Codex/Claude session, use this file as the continuity source.

Tell the assistant/agent:

~~~text
Read docs/MASTER_PLAN.md first.
Then inspect the latest master commit and open PRs.
Do not redesign the project from scratch.
Resume from the first incomplete milestone in the "Recommended immediate PR sequence".
Preserve the architecture principles and hard safety invariants.
Keep repository artifacts in English.
Do not hard-code a model per agent role.
Do not bypass CI, workspace integrity, declared-file scope, or quality gates.
~~~

At the time this file was created:

~~~text
Latest completed milestone: P2a
PR #63: merged
Merge commit: 2c116ff717505dd7af2ad8f280dfd78197a3c936
Next milestone: P2b — Lazy baseline integration in quality.Loop
~~~

Before beginning work, always verify that this checkpoint is still current.

---

# 19.5 Integrated implementation checkpoint — master

The previously stacked implementation from PR #66 through PR #119 has been integrated into `master` through PR #120.

Current master checkpoint:

~~~text
merge commit: 15cc2bed75a48f7aa63ecb920577d75bb61b4ca0
integration PR: #120
integration CI: passed
master push CI: passed
open stacked PRs: none
~~~

## Functional capability now on master

AI Engineering OS now has a real backend-first multi-agent engineering path:

~~~text
user task
  -> classify intent/capabilities
  -> durable run
  -> deterministic specialist DAG
  -> Researcher / Architect collaboration
  -> typed handoffs + bounded inbox
  -> isolated source-writer stages
  -> deterministic source integration
  -> tests-only QA overlay
  -> durable integration checkpoint
  -> run-wide baseline-aware Quality
  -> independent review/correction
  -> persisted promotion candidate
  -> explicit human approval
  -> promotion into original project
~~~

The original project remains protected until:

~~~text
ai run promote <run-id> --approve
~~~

## Durable human controls now on master

Read-only and mutation runs have explicit durable controls:

~~~text
ai run cancel <run-id>
ai run pause <run-id>
ai run resume <run-id>
ai run retry <run-id> <task-id>
ai run recover <run-id>
~~~

Safety boundaries:

- read-only retry operates as a child run and does not rewrite the failed parent;
- mutation pause happens only at durable integration checkpoints;
- mutation resume restores only integrity-checked checkpoints;
- active runners use a durable lease/heartbeat;
- stale leases are never reclaimed automatically;
- crash recovery requires an explicit command and stale-heartbeat evidence;
- mutation recovery requires a valid source/final integration checkpoint;
- losing lease ownership cancels the active execution context.

## Control Plane surface now on master

The backend can expose:

- recent run index;
- projected run/task/agent state;
- task DAG edges and dependencies;
- file/mutation scopes;
- ordered event history;
- resumable SSE stream;
- typed messages;
- immutable artifacts;
- Quality state;
- promotion status;
- pause/cancel request state;
- latest durable checkpoint phase;
- retry lineage;
- deterministic safe human controls;
- stale-run recovery readiness.

Important endpoints include:

~~~text
GET /runs
GET /runs/<run-id>/state
GET /runs/<run-id>/events
GET /runs/<run-id>/stream
GET /runs/<run-id>/messages
GET /runs/<run-id>/artifacts
GET /runs/<run-id>/promotion
GET /runs/<run-id>/controls
~~~

The visual Promix-like experience remains intentionally a client of this backend, not the source of coordination truth.

## Current product level

The project has crossed these earlier completion levels:

- safe single-agent foundation: complete enough for current architecture;
- baseline-aware Quality: implemented;
- durable/resumable run foundation: implemented;
- collaborative agents: implemented and active;
- backend multi-agent Control Plane: implemented and active;
- controlled long-running recovery: first production-safe foundation implemented.

Still incomplete for the full product vision:

- broader language/toolchain Quality normalization;
- stronger contract/architecture verification;
- long-run recovery/observability soak coverage;
- deeper durable memory lifecycle/team learning;
- broader external tool/MCP mesh hardening;
- polished human visual/IDE client;
- final production-grade release matrix and threat-model closure.

## Recommended next implementation from master

Proceed in this order unless a new explicit architecture decision changes it:

1. cross-language Quality providers and normalized failure identity;
2. contract/architecture verification;
3. long-run recovery/observability soak tests;
4. memory/team-learning hardening;
5. external tool/MCP mesh hardening;
6. optional visual Control Plane client;
7. final production release matrix and threat-model closure.

Snapshot creation, snapshot-assisted state projection, independently verifiable event archives, explicit archive-backed event compaction, and typed lifecycle/coordination payload contracts are now implemented on master and are no longer pending work.

Compaction remains explicit and fail-closed. Historical bytes remain reconstructable from immutable compacted-history generations after individual event files are removed.

Keep the current invariants:

- deterministic first, AI second;
- provider-independent model policy;
- no model pinning by role;
- no automatic permission escalation;
- no automatic promotion;
- no automatic stale-lease takeover;
- original repository protected by PreImage/integrity checks;
- event history remains auditable across snapshots/archives;
- independent Quality/review before promotion.

# 20. Living roadmap rule

This file must evolve with the implementation.

After every meaningful milestone:

1. mark the milestone complete;
2. update the current checkpoint;
3. record the merged PR number and merge commit;
4. adjust the next milestone;
5. add newly discovered architectural constraints;
6. remove obsolete implementation details;
7. preserve the long-term vision unless an explicit architecture decision changes it.

The repository, not a chat transcript, is the durable source of truth for the project plan.


## Verified engineering checkpoint — 2026-11-11 (PRs #183–#187)

This supersedes older "resume here" implementation pointers for the current
post-RC development sequence. Historical milestone descriptions remain in this
document for continuity.

### Merged milestone evidence

| PR | Scope | Verified merge SHA |
| --- | --- | --- |
| #183 | Prevent sequential replay of approved GitHub PR creation using a durable pre-mutation event | `68578b2ad31b8665e1d40c39fab5a1a2e0a0611e` |
| #184 | Atomic, immutable mutation claim prevents concurrent duplicate creation across processes sharing an approval store | `f3454bededa35ff1d2deae1a7dccd93a3d5b08c9` |
| #185 | Validate external GitHub PR result URLs against the exact approved repository and GitHub origin | `620066c613f628c86048d5ab6a1200c466df7118` |
| #186 | Python pytest/JUnit evidence fails closed on invalid roots, duplicate failure identities, inconsistent counts and contradictory exit status | `cb5ae99dedf2a7725d5a8abbfacaa3aa41c0ccab` |
| #187 | Reject symlinked original/staged OpenAPI paths before contract verification reads | `f6459b4860351bd9e0778d19e832f445ffd063db` |

All five PRs passed the applicable GitHub Actions Go matrix on Ubuntu,
Windows and macOS before merge. #183 and #184 additionally passed their
applicable release artifact workflows. A skipped tag-only release publishing
job on a PR is intentional; it is not a public release.

### Current boundaries and limitations

- Creating a GitHub PR requires a matching human grant, durable audit evidence
  and a one-time atomic mutation claim. Ambiguous outcomes are *not*
  automatically retried. Concurrent claims require all processes to use the
  same storage location and a filesystem supporting exclusive creation.
- Only the approved GitHub.com repository may be accepted as the result of a
  GitHub PR creation operation. Enterprise hosts require explicit contracts.
- Pytest JUnit evidence with contradictory or ambiguous structure/counts
  cannot authorize a baseline-aware success classification.
- OpenAPI contract file and parent-directory symlinks fail closed. The
  pre-read check is **not** a race-free descriptor-based no-follow read if an
  untrusted process can mutate the workspace concurrently.
- Green CI proves only the tested behavior and environments. It does not
  imply that the entire long-term product vision is complete.

### Next prioritized engineering milestones

1. Extend cross-language structured Quality evidence integrity and
   deterministic failure comparison, including more adversarial fixtures.
2. Harden contract/architecture input reads against concurrent path swapping
   where the supported operating systems permit atomic no-follow access.
3. Complete external-tool/MCP runtime bindings with provider-verified
   operations; never equate a registry declaration with authorization.
4. Expand safe interactive browser/Playwright integration only after a real
   transport/operation contract and explicit human approval are verifiable.
5. Improve Control Plane and IDE usability, end-to-end product scenarios and
   release/readiness evidence before declaring a stable production version.

**Resume rule:** Inspect live `master`, open PRs and CI first. Continue from
the earliest incomplete milestone above; do not assume a historical checkpoint
is still current. Always preserve human approval and green CI before merge.


### In-progress Quality evidence milestone — nested JUnit counts

- Proposed change: reject negative/impossible JUnit counts and contradictions between aggregate suite totals and fully declared child-suite totals.
- Regression fixtures cover nested failure/test count mismatches and invalid totals.
- This milestone is **not complete** until the corresponding PR has passed required Ubuntu, Windows, macOS and applicable race/package checks and has been merged from its verified head SHA.
- After merge, continue cross-language Quality hardening and then no-follow contract reads, external tool runtime contracts and product UX per the prioritized checkpoint above.


### Architecture import coverage — dynamic module loading (proposed)

- The architecture import scanner now recognizes literal JavaScript/TypeScript `import("module")` expressions, including JS/JSX/TS/TSX files, alongside static imports and CommonJS `require`.
- Regression tests verify forbidden literal dynamic imports are reported and allowed imports remain accepted.
- This is lexical detection for literal module specifiers, **not** a full JS/TS parser. Computed dynamic imports and aliases are outside this milestone; no claim of complete enforcement.
- Treat this checkpoint as pending until exact-head Ubuntu, Windows, macOS and Ubuntu race checks pass and the PR is merged.


### Dynamic import coverage follow-up — bundler comments (proposed)

- Extend the lexical JS/TS architecture check to recognize a literal dynamic import preceded by an optional block comment inside `import(/* chunk */ "module")`, including multiline comments.
- Add positive and negative regression tests across JS/JSX/TS/TSX.
- This remains a lexical heuristic, not complete JS/TS parsing or computed-import enforcement. No race-free filesystem guarantees are implied.
- Merge only after exact-head green CI on Ubuntu, Windows and macOS, including Ubuntu race, build and vet.


### Verified continuation — PRs #201–#202

- PR #201 merged at `0567ebc0487053e362c5eb57ae23aca7b8926e1e`: literal JS/TS dynamic import architecture checks.
- PR #202 merged at `462e14391883093c931965ed83ef60bc817ad60b`: single bundler block comment before a literal dynamic import.
- Both passed exact-head Ubuntu/Windows/macOS tests and Ubuntu race before merge. The earlier "proposed" entries above are historical, not outstanding PRs.
- Current proposed follow-up: reject consecutive block-comment evasion of literal dynamic import rules, with cross-extension regressions. This is still lexical scanning, not a JS/TS parser or a complete no-follow filesystem guarantee.
- After green CI and merge, prioritize stronger parser-backed architecture verification, atomic contract reads, and external-tool runtime authorization as described above.


### Architecture source-tree symlink guard (pending CI)

- The architecture scanner must reject symlinked directories instead of silently skipping their contents during `filepath.WalkDir`; a linked `src/` tree could otherwise hide forbidden imports.
- A regression test creates a linked source directory with a forbidden import and requires fail-closed behavior.
- This is a pre-scan guard only. It is not race-free descriptor-relative traversal and does not solve concurrent workspace mutations.
- Do not merge until exact-head Ubuntu/Windows/macOS tests, Ubuntu race, build and vet are green and the PR is conflict-free.


### OpenAPI discovery — linked directory fail-closed guard (pending CI)

- Reject symlinked directories during original OpenAPI/Swagger contract discovery; `filepath.WalkDir` otherwise skips linked contract trees silently.
- Add cross-platform regression that places `openapi.yaml` inside a symlinked contract directory, with permission-based skip on restricted Windows environments.
- This is not atomic descriptor-relative no-follow traversal and does not protect against concurrent filesystem swaps.
- Merge only after exact-head Ubuntu/Windows/macOS tests, Ubuntu race, build and vet pass.


### Architecture literal dynamic imports — line-comment gaps (pending CI)

- Recognize JS/JSX/TS/TSX literal `import(` calls with JavaScript `//` line comments and mixed block comments before the quoted module specifier.
- Add forbidden/allowed regression fixtures. This remains lexical detection, not full parser-backed JavaScript/TypeScript import analysis.
- Merge only with exact-head green Ubuntu, Windows, macOS, Ubuntu race, build and vet.

### Verified integration checkpoint — 2026-11-11

- PR #205: architecture directory-symlink fail-closed guard; merged to master after green cross-platform CI.
- PR #206: OpenAPI discovery directory-symlink fail-closed guard; merged as `b6e854c928daaf9fb68bf94f53679712bb9aa0f4` after green Ubuntu/Windows/macOS, Ubuntu race, build and vet.
- PR #207: literal dynamic imports with line comments; merged as `301086c3c71ce1aec296838bf7ae7a44338bc8d7` after exact-head `6cc7a67db36da6ab4218e277bb9c861584d978dc` passed Ubuntu/Windows/macOS tests, Ubuntu race, build and vet (run 43137857412).
- Prior section headings saying "pending CI" describe historical implementation checkpoints, not open PRs.
- Next engineering priority: parser-backed JavaScript/TypeScript import analysis and/or atomic no-follow file reads. Avoid claiming the existing regex scanner or pre-read filesystem checks are race-free. Scope and test one small cross-platform slice before merging.


### Architecture dynamic imports — trailing comments (pending CI)

- Detect forbidden literal dynamic import specifiers when JavaScript block or line comments occur between the quoted module and the closing `) ` token.
- Add JS/JSX/TS/TSX negative regressions and an allowed-import positive fixture.
- Lexical check only: not a complete JS/TS parser; do not claim computed imports are covered.
- Merge only after exact-head green Ubuntu/Windows/macOS tests, Ubuntu race, build and vet.


### 2026-11-12 — dynamic import call-boundary comment guard (pending CI)

- Baseline master `57c77c85bf60f3344c47cdfbd4f0a20d1c7e0890` after verified PR #209 merge. No open PRs at selection.
- Detect literal JS/JSX/TS/TSX dynamic imports with mixed block and line comments between `import` and `(`; add forbidden and allowed regressions.
- This remains a lexical guard, not parser-backed JS/TS analysis; computed imports and concurrent filesystem swaps remain outside this milestone.
- Require exact-head Ubuntu/Windows/macOS tests, Ubuntu race, build and vet; do not merge before green CI or override human approvals.


### 2026-11-12 — Go test JSON unknown actions (PR #211)

- Reconciled PR #211 with master `d7b53ede872a5d2303ab5f2ff117dcd1e91adc3b` using an explicit two-parent merge commit and master-based tree. Diff against master contains only `integrations/localverify/gotest.go` and its regression tests before this checkpoint.
- Unrecognized Go test JSON actions now make evidence partial rather than complete; known actions retain complete classification.
- CI gate: exact-head Ubuntu/Windows/macOS tests, Ubuntu race, build and vet; no merge until verified green and conflict-free.
- This does not validate all future Go JSON event schema changes.


### 2026-11-12 — Go test JSON lifecycle package integrity (pending CI)

- Baseline master `4342afc04333e80b50a63e3267fa87304e9d93fd` after verified PR #211 merge.
- Require nonempty Package for Go test JSON start/run/pass/fail/skip lifecycle events. Unscoped lifecycle events now make mixed streams partial, preventing attribution to an empty package.
- Regression fixtures cover all five unscoped lifecycle actions and a valid scoped run/pass sequence.
- Build events remain separately scoped by ImportPath; this slice does not validate all JSON event fields or event ordering.
- Merge only after exact-head Ubuntu/Windows/macOS tests, Ubuntu race, build, vet, and conflict-free PR. No release or deployment.

### 2026-11-12 — Go test JSON build identity integrity (pending CI)

- Baseline master `d3f9d7b6a6a1f72dc28e08a7f666ef55f4b1d572` after verified PR #212 merge; no open PRs at selection.
- Reject `build-output` and `build-fail` JSON events with missing/blank ImportPath or an empty base path, marking otherwise-valid streams partial instead of creating unattributable build identities.
- Regression tests cover missing, whitespace and empty base paths for both actions, plus a correctly scoped build failure.
- This does not prove complete Go event schema or event ordering. Merge only after exact-head Ubuntu/Windows/macOS tests, Ubuntu race, build, vet, and no conflicts. No release or deployment.


### 2026-11-12 — Go test JSON output identity integrity (pending CI)

- PR #213 merged to master as `f2b7ba6be56243d52a4b615532792cf5b8b2e444` after exact-head Ubuntu/Windows/macOS tests, Ubuntu race, build and vet passed (run 43176299686).
- Baseline master `f2b7ba6be56243d52a4b615532792cf5b8b2e444`, with no open PRs at selection.
- Require a nonempty Package for ordinary `output` events, preventing unattributed output from entering test/package signatures while marking mixed streams partial.
- Regression tests cover missing/blank Package and valid scoped output. Build output remains scoped by ImportPath.
- This does not prove the entire Go JSON event schema, ordering, or complete quality-evidence integrity.
- Merge only after exact-head Ubuntu/Windows/macOS tests, Ubuntu race, build and vet pass, no conflicts and all required approvals. No release or deployment.


### 2026-11-12 — Go test JSON package-start scope integrity (pending CI)

- Baseline master `b9196c6c23139e358e909e516b748ba2263c85c9` after PR #214 merged; no open PRs when selected.
- Reject `start` actions containing a nonempty Test: Go package-start events are package-scoped, and malformed test-scoped starts must not authorize complete evidence.
- Add regression coverage for malformed test-scoped starts and legitimate package-scoped starts.
- PR #214 is merged (its earlier checkpoint says pending CI historically); the current milestone remains pending exact-head Ubuntu/Windows/macOS tests, Ubuntu race, build, vet, conflict and approval checks.
- This is a narrow Go JSON lifecycle validation improvement, not proof of complete schema or ordering integrity. No release or deployment.


### 2026-11-12 — Go JSON test-run identity integrity (pending CI)

- PR #215 merged to master at 9019549c056ee884ac3c3556250d0c1ed0281ee5 after exact-head Ubuntu/Windows/macOS tests/build/vet and Ubuntu race passed.
- Reject run events lacking a nonblank Test identity; a Go run event must refer to a test or subtest, not a package-level run.
- Add regressions for missing/whitespace Test and a valid test-scoped run/pass lifecycle.
- This milestone is pending CI, exact-head mergeability and any required human approval. No release, deploy, visibility changes or bypasses.

### 2026-11-12 — Go JSON terminal test identity (pending CI)

- After PR #216 merged to master (`9e33eb972b6eb42d03b6f1a4710bd559c5549257`), no PRs were open at selection.
- Reject whitespace-only `Test` identities on test-scoped `pass`, `fail`, and `skip` events. Such events must not generate bogus failure identities or count as complete evidence.
- Regression coverage includes malformed whitespace identities and valid test-scoped terminal actions.
- This is a narrow schema-integrity check, not full lifecycle/order verification. Require exact-head Ubuntu/Windows/macOS tests, Ubuntu race, build, vet, applicable release checks, no conflicts and human approvals before merge. No release, deployment or visibility change.


### 2026-11-12 — Output test identity integrity (PR pending)

- Baseline: `master` at `2b851809e6f7edc11adc457195eb4d02ae6b5644`, PR #217 merged, no open PRs at start.
- Guard: reject `go test -json` output events whose nonempty `Test` is whitespace-only; mark evidence partial rather than attributing output to a malformed test identity.
- Regression coverage: malformed whitespace-only identity and valid test-scoped output.
- Integration: pending GitHub Actions on Ubuntu (including race), Windows and macOS; no merge before checks, approvals and SHA verification.


### 2026-11-12 — Invalid-only Go JSON parse confidence (pending CI)

- Baseline `master` `e3e445ee6f9ba1cb8d25bd6c6805deea50baf404` after PR #218 merge; no open PRs at selection.
- A nonempty `go test -json` stream containing only invalid events must yield partial parse confidence on a successful command exit, rather than being mistaken for no attempted parse.
- Preserve failed confidence for nonzero exit without valid events and not-attempted confidence for genuinely empty successful output.
- Regression tests cover invalid JSON, unknown action, unscoped output, and failed exit.
- Integration pending exact-head Ubuntu/Windows/macOS tests, Ubuntu race, build, vet, applicable release workflows, conflict checks and required human approvals. No release, deployment, visibility changes or bypasses.


### 2026-11-12 — Build JSON event test-scope integrity (PR pending)

- Baseline master `55ff78a758ff2036440b71ed8b4981cb5d556b8a` after PR #219 merged; no open PRs at selection.
- Reject `build-output` and `build-fail` events with a nonempty `Test` field: build events are scoped to `ImportPath`, not an individual test.
- Regression coverage checks both malformed actions, absence of fabricated failures and suppression of invalid build output.
- Prior checkpoint for PR #219 is historical: its exact-head CI passed and PR was merged as `55ff78a7`.
- This change remains pending exact-head Ubuntu/Windows/macOS tests, Ubuntu race, build, vet, mergeability and applicable approvals. No release, deployment or visibility changes.


### 2026-11-12 — FailedBuild identity integrity (pending CI)

- Baseline master `8e707585e5a4504384d88c1673a18841ed6807f9`; PR #220 merged with exact-head Ubuntu/Windows/macOS checks and Ubuntu race green, no open PRs at selection.
- For Go package `fail` events with nonempty `FailedBuild`, reject blank or empty-base import paths. Malformed failed-build attribution must yield partial confidence, not a fabricated build identity.
- Regressions cover whitespace-only and empty-base `FailedBuild`, plus valid build attribution.
- This is a narrow cross-language Quality evidence integrity improvement, not completion of nested JUnit evidence or the broader product milestones.
- Require exact-head Ubuntu/Windows/macOS tests, Ubuntu race, build, vet, applicable release checks, conflict-free merge and human approvals before integration. No release, deployment or visibility changes.


### Multi-agent module block 1 — coordination contracts (in progress)

- Baseline `master` `9cb0ea581e4fe1136c233fc621c42b39a0d19265`; PR #222 CI still pending macOS at branch creation.
- Introduced provider-independent `core/coordination` task graph and typed inter-agent message contracts, with deterministic cycle/missing dependency checks, dependency-ready selection, and scoped sender/recipient/artifact validation.
- This is the first **module slice**, not a completed durable message bus or runtime scheduler. Next slices: append-only message persistence with run scoping and replay; task state transitions and ownership; orchestrator integration; Control Plane projections and end-to-end scenarios.
- CI, review, and exact-head merge checks remain mandatory; do not infer runtime authority from these contracts.
