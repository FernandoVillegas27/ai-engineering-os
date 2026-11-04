# AI Engineering OS — Master Plan and Continuity Roadmap

> Canonical continuity document for the project.
>
> Purpose: preserve the full product vision, current state, execution order, architectural constraints, and acceptance criteria so development can continue safely across chats, IDE sessions, models, and contributors.

Last updated: 2026-11-03

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

### Latest completed milestone

PR #63 — P2a Baseline Integrity Foundation — merged.

Merge commit:

~~~text
2c116ff717505dd7af2ad8f280dfd78197a3c936
~~~

P2a added:

- immutable `staging.PreImage`;
- baseline provider;
- original/pre-image integrity checks;
- staged authorized-change checks;
- memoization with integrity rechecks;
- explicit `IntegrityError`;
- Go 1.23 compile stderr handling;
- compile failures as unconditional blocking failures;
- Windows and Linux validation.

### Immediate next milestone

**P2b — integrate lazy baseline behavior into `quality.Loop`.**

Do not skip directly to multi-agent communication.

The quality foundation must become production-active first.

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

# 19.5 Active implementation stack — validation checkpoint

This section records the current **unmerged** implementation stack so a future session can distinguish completed work on `master` from work that is implemented and awaiting human validation.

At this checkpoint, `master` still has P2a as the latest functional merged milestone. The following PRs are intentionally stacked and remain open for ordered review.

| PR | Phase / capability | Validation state |
|---|---|---|
| #66 | P2b — lazy baseline in Quality Loop | CI passed |
| #67 | P2c — structured quality outcomes | CI passed |
| #68 | P3a — promotion dry-run | CI passed |
| #69 | P3b — approval-gated apply + rollback | CI passed |
| #70 | P4 — durable run state | CI passed |
| #71 | durable run CLI | CI passed |
| #72 | P5 — Engram project save/recall | CI passed |
| #73 | P6 — immutable agent artifacts | CI passed |
| #74 | P7 — typed inter-agent message bus | CI passed |
| #75 | P8 — dependency-aware DAG scheduler | CI passed |
| #76 | P9 — concurrent execution coordinator | CI passed |
| #77 | P9.5 — durable Control Plane event stream/projector | CI passed |
| #78 | Control Plane CLI inspection | CI passed |
| #79 | integrated durable coordinated read-only run | CI passed |
| #80 | deterministic specialist collaboration DAG | CI passed |
| #81 | automatic durable specialist handoffs | CI passed |
| #82 | bounded receiver inbox context | CI passed |
| #83 | loopback JSON/SSE live Control Plane API | CI passed |
| #84 | projected task-graph edges and scopes | CI passed |
| #85 | stale append-lock recovery | CI passed |
| #86 | typed ArchitectureDecision/APIContract/VerificationReport payloads | CI passed |
| #87 | isolated per-task mutating workspaces | CI passed |
| #88 | deterministic multi-workspace integration plan | CI passed |
| #89 | isolated integration workspace materialization + frozen-result TOCTOU protection | CI passed |
| #90 | run-wide baseline-aware integration Quality Gate | CI passed |
| #91 | bridge promotable integration to explicit promotion boundary | CI pending |

Review/merge order must remain:

~~~text
#66 -> #67 -> #68 -> #69 -> #70 -> #71 -> #72 -> #73 -> #74
-> #75 -> #76 -> #77 -> #78 -> #79 -> #80 -> #81 -> #82
-> #83 -> #84 -> #85 -> #86 -> #87 -> #88 -> #89 -> #90 -> #91
~~~

Do not merge later PRs before their stacked bases unless the branch is deliberately rebased and the resulting diff is revalidated.

## Functional capability reached by the open stack

If the stack through #90 is validated and merged, AI Engineering OS has both the first complete read-only multi-agent collaboration path and the safety pipeline required before write-capable coordination is enabled:

~~~text
user task
  -> classification
  -> deterministic specialist plan
  -> durable run
  -> role-aware DAG
  -> bounded concurrent coordinator
  -> agent execution
  -> immutable handoff artifact
  -> typed message to dependent agent
  -> bounded receiver inbox
  -> dependent agent execution
  -> durable Control Plane events
  -> CLI / JSON / SSE observation
~~~

The Control Plane can expose:

- real run state;
- agents and their task state;
- DAG edges;
- task ownership;
- dependency state;
- mutating/file-scope metadata;
- message/artifact counts;
- quality state;
- approvals;
- runtime/model summaries;
- ordered event history;
- resumable SSE events.

This is the first functional foundation for the Promix-like experience described earlier. It is intentionally backend-first; no visual UI is required for the coordination semantics to work.

The open stack now also contains the write-safety pipeline:

~~~text
per-task isolated writable workspace
  -> immutable PreImage
  -> inspected frozen ResultImage
  -> deterministic disjoint integration plan
  -> isolated integration workspace
  -> source-hash TOCTOU validation
  -> run-wide baseline-aware Quality Gate
  -> Promotable=true
  -> promotion candidate
  -> promotion dry-run
  -> explicit approval required
  -> apply
~~~

## Current safety boundary

The main coordinated command remains **read-only**.

The components needed for safe multi-agent writes now exist through #91, but write-capable stage runtimes are not yet wired into `ai run execute`.

That wiring must remain opt-in and must route every mutating task through the isolated task/integration/Quality pipeline above.

Promotion remains a separate explicit approval boundary.

## Recommended next implementation after this stack

After #91 is green and the stack is reviewed, the next high-value milestones are:

1. adapt existing Cursor/Codex stage runtimes to the coordinated mutating-task pipeline behind an explicit opt-in policy;
2. generate/freeze proposals before mutating task execution so exact file ownership is known before integration;
3. persist integration/quality/promotion lifecycle events in the Control Plane;
4. add Control Plane snapshots/compaction for long runs;
5. wire real pause/resume/cancel/retry human controls into active coordinator executions;
6. add dedicated typed schemas for remaining high-value artifacts/handoffs;
7. only then consider a visual web/IDE/3D client.

---

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
