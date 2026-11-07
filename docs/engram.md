# Engram Integration

## Purpose

Engram is the first memory provider integrated with AI Engineering OS.

The integration remains outside the core:

~~~text
core/memory
    ↓ provider contract
integrations/engram
    ↓
Engram binary
    ↓
MCP / CLI / HTTP
    ↓
SQLite + FTS5
~~~

This keeps AI Engineering OS independent from Engram internals while still using Engram as the initial persistent-memory implementation.

## Why Engram

Engram is agent-agnostic and exposes the same memory system through MCP, CLI and HTTP.

For normal agent usage, AI Engineering OS models the preferred MCP entrypoint as:

~~~text
engram mcp --tools=agent
~~~

The adapter does not embed Engram's Go packages.

## Status probe

The integration can deterministically check whether Engram is available.

It uses:

~~~text
engram version
engram doctor --json
~~~

This lets future commands such as `ai doctor` report whether persistent memory is actually ready instead of assuming it is installed.

## Runtime setup

Engram supports setup for multiple coding-agent hosts.

The integration currently recognizes setup targets such as:

- Cursor
- Claude Code
- Codex
- OpenCode
- Gemini CLI
- VS Code Copilot
- Windsurf
- Kiro
- Qwen
- Pi

The adapter can produce the matching Engram command:

~~~text
engram setup cursor
engram setup codex
engram setup claude-code
~~~

AI Engineering OS does not run these mutations automatically in this step.

## Project memory

The Project contract already contains:

~~~yaml
memory:
  provider: engram
  namespace: my-project
~~~

Engram itself is project-aware. The future runtime will use that project identity when restoring and persisting engineering memory.

## Memory protocol

Engram memory should be curated engineering memory, not a transcript dump.

The intended runtime behavior is:

~~~text
session starts
    ↓
confirm current project
    ↓
recover relevant context
    ↓
work
    ↓
save durable decisions / discoveries / fixes
    ↓
session summary
~~~

Exact memory tool calls remain an Engram integration concern rather than being hard-coded into the core Agent contract.

## Installation

Engram is an external runtime dependency.

The repository integration does not install software on a developer workstation by itself.

Once this PR is merged, the next local setup step will install the current stable Engram binary and configure the coding agents we actually use.

## Current boundary

This step does not yet:

- install Engram on a workstation;
- change Cursor or other agent configuration;
- write memories;
- retrieve memories;
- enable Engram Cloud;
- add Context7;
- integrate Gentle-AI.

It establishes the provider boundary and deterministic operational checks first.


## Project memory lifecycle v1

The integration now exposes the first real project-scoped read/write contract.

Core requests are provider-independent:

~~~text
Save(project, title, content, type, topic_key)
Recall(project, query, limit)
~~~

Supported curated memory types are:

- `decision`
- `architecture`
- `bugfix`
- `pattern`
- `config`
- `discovery`
- `learning`

The Engram adapter maps these contracts to the current Engram CLI:

~~~text
engram save <title> <content> --project <project> --type <type> --scope project [--topic <topic_key>]

engram search <query> --project <project> --limit <N>
~~~

Project is always explicit at the AI Engineering OS boundary. The adapter does not rely on the provider guessing the current repository for these operations.

Recall is bounded to at most 20 results and defaults to 5.

### Curated memory rule

This integration is for durable engineering knowledge, not transcript dumping.

Callers should save concise decisions, architecture facts, fixes, conventions, configuration discoveries and reusable learnings. Raw provider conversations and arbitrary source files are not memory records.

### Current boundary

The provider can now save and recall project memory, but orchestration does not yet automatically inject recalled context or automatically persist memory candidates at the end of every run.

That lifecycle wiring is the next memory step.


## Automatic run memory lifecycle

`ai run execute` now supports:

~~~text
--memory auto|engram|off
~~~

Default: `auto`.

Behavior:

- `auto`: use Engram only when its deterministic status probe reports it available and healthy;
- `engram`: select the Engram provider explicitly, while memory operations remain best-effort and non-blocking;
- `off`: do not probe, recall, or persist project memory.

Dry-run execution does not invoke project memory.

### Recall boundary

Before real execution, the run recalls bounded project-scoped context using:

- the stable project ID as namespace;
- the current task as query;
- at most 5 results by default;
- at most 8 KiB of recalled provider context.

Recalled context is labeled explicitly as **untrusted historical evidence**.

It is combined with current-run inbox/handoff context without replacing either source, and it cannot change:

- task;
- permissions;
- system/safety constraints;
- tool availability;
- runtime/model policy.

### Persistence boundary

Successful runs may persist two categories of curated knowledge:

1. selected durable artifacts:
   - architecture decisions;
   - API contracts;
   - explicit decisions;
   - research notes;
2. one bounded run-learning summary built from completed task summaries.

Transient handoffs, questions, arbitrary messages, raw prompts, transcripts, source files, and failed-run summaries are not automatically written as durable memory.

Artifact memories use stable topic keys derived from type + normalized summary, so repeated equivalent knowledge converges instead of producing unbounded duplicates.

Run learning uses a stable project namespace and task/intent-derived topic key.

### Failure behavior

Memory is an optional context provider, never the coordination source of truth.

Provider unavailable/unhealthy, recall errors, and save errors become observable warnings. They do not turn an otherwise valid engineering run into a failed run.

The durable run state, Control Plane events, Quality gates, permission envelopes, checkpoints, and promotion approval remain authoritative.


## Governed memory authority

Automatic run memory now passes through the AI Engineering OS governance ledger before any optional Engram mirror write.

The governed local ledger is the authority for automatic recall. Engram is an optional mirror/integration, not the lifecycle authority.

This means:

- governed memories are inspectable with `ai memory list/show`;
- delete and supersede affect future automatic recall immediately;
- expired/superseded/deleted records are excluded from automatic context;
- common secrets are redacted before local persistence and before the Engram mirror receives content;
- provenance and retention are stored independently of Engram capabilities;
- provider-native records that are not present in the governed ledger are not injected automatically.

See [memory-governance.md](memory-governance.md) for the lifecycle and retention policy.
