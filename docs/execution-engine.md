# Execution Engine v1

## Purpose

Execution Engine v1 converts an orchestration plan into explicit work units under a safe permission envelope.

It supports two execution paths:

- dry-run validation without invoking a runtime;
- real read-only analysis through an approved runtime.

~~~text
Execution Plan
      ↓
Work Unit
      ↓
Read-only permission envelope
      ↓
Dry-run Executor OR Read-only Runtime Executor
      ↓
Execution Result
~~~

## Work Unit

A work unit contains:

- stable work-unit ID;
- original task;
- project root;
- execution intent;
- selected agent;
- assigned capabilities;
- enabled skills;
- logical tools;
- effective permissions;
- execution mode.

Candidate skills are deliberately excluded from executable work units.

Only skills already marked as enabled by the resolver may enter execution.

## Permission narrowing

Execution Engine v1 always narrows permissions to read-only.

Even when the selected agent normally has:

~~~yaml
source_code: write
tests: write
git_commit: ask
git_push: ask
~~~

the effective v1 work-unit envelope becomes:

~~~yaml
source_code: read
tests: read
git_commit: deny
git_push: deny
secrets: deny
production: deny
~~~

An execution layer may reduce permissions granted by an agent contract. It must never silently expand them.

## Dry-run executor

The dry-run executor validates the work unit and returns an execution result without invoking a runtime provider.

~~~yaml
status: dry-run
executed: false
changes: []
~~~

This establishes the boundary between orchestration and runtime execution before source mutation is introduced.

## Safety invariants

Execution Engine v1 guarantees:

- no source writes;
- no test writes;
- no Git commit;
- no Git push;
- no secret access;
- no production access;
- no candidate skill execution;
- no runtime agent invocation in dry-run mode;
- real runtime execution only under the read-only envelope.

## Read-only runtime executor

The runtime executor invokes an approved runtime only after validating the same read-only permission envelope.

A successful runtime analysis returns:

~~~yaml
status: completed
executed: true
runtime: codex-cli
changes: []
summary: ...
findings:
  - severity: ...
    title: ...
    detail: ...
evidence:
  - path: ...
    detail: ...
~~~

The first concrete runtime is Codex CLI.

## Tool boundary

Logical tools in the orchestration plan are metadata until a runtime/tool resolver proves they are actually available.

Candidate skills do not contribute executable tools. Only enabled skills may add tool requirements to a plan.

## CLI

Dry-run:

~~~powershell
go run ./cmd/ai execute . --task "..." --dry-run
~~~

Real read-only analysis:

~~~powershell
go run ./cmd/ai execute . --task "..." --runtime codex
~~~

## Next step

Future runtimes such as Claude or Copilot can implement the same Runtime contract without changing Work Units or the permission envelope.
