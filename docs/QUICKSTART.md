# Quickstart

This quickstart starts with a credential-free deterministic path and only then enables external AI runtimes.

## Prerequisites

Required:

- Git;
- Go version declared by `go.mod`.

Optional, depending on the workflow you want:

- Codex CLI, authenticated through its normal local setup;
- Cursor CLI (`agent`), authenticated through its normal local setup;
- Engram for an external memory mirror;
- Node.js / `npx` for Context7 MCP launcher health;
- Gentle-AI for runtime projection workflows.

AI Engineering OS does not require provider credentials to build, inspect a repository, run deterministic dry-runs, execute tests, or open the Control Plane against existing durable runs.

## Build

From a fresh checkout:

~~~sh
go build -o ai ./cmd/ai
~~~

Windows PowerShell:

~~~powershell
go build -o ai.exe ./cmd/ai
~~~

Verify the binary:

~~~sh
./ai version
~~~

## Diagnose the environment

~~~sh
./ai doctor .
~~~

Optional providers may appear as warnings. A missing optional provider does not make the core installation invalid.

## Inspect a project

From the target repository:

~~~sh
/path/to/ai inspect . --write
~~~

This creates the AI Engineering OS project context under `.ai/project.yaml`.

Review that file before using autonomous workflows.

## Plan without executing

~~~sh
/path/to/ai plan . --task "review the checkout architecture"
~~~

The deterministic classifier and registry resolve capabilities, agents, skills and logical tools without executing a model.

For genuinely ambiguous natural-language tasks, `--ai` enables the configured classification fallback.

## Credential-free coordinated dry-run

~~~sh
/path/to/ai run execute . \
  --task "review the checkout architecture" \
  --dry-run \
  --memory off
~~~

A durable run is created and can be inspected with:

~~~sh
/path/to/ai run list
/path/to/ai run control <run-id>
/path/to/ai run events <run-id>
~~~

## Control Plane UI

Start the local loopback server:

~~~sh
/path/to/ai run serve
~~~

Default:

~~~text
http://127.0.0.1:8765/ui/
~~~

The embedded UI is a read-only client of the existing Control Plane state/events/messages/artifacts/promotion/control APIs. It is not a second source of truth.

## Real read-only execution

Codex:

~~~sh
/path/to/ai run execute . \
  --task "review the checkout architecture" \
  --runtime codex
~~~

Cursor:

~~~sh
/path/to/ai run execute . \
  --task "review the checkout architecture" \
  --runtime cursor
~~~

Keep the default provider-independent model policy unless you have a concrete reason to override it.

## Staged multi-agent mutation

A mutating coordinated run operates in isolated task workspaces and does not promote changes to the original repository automatically.

Example:

~~~sh
/path/to/ai run execute . \
  --task "implement the requested change and add tests" \
  --runtime auto \
  --multi-agent-write \
  --max-concurrency 2
~~~

The run must pass deterministic Quality, independent review, scope/integrity checks and contract/architecture gates before a promotion candidate can be created.

Promotion still requires explicit approval.

## Resource budgets

Real runs default to bounded provider usage.

Override only when justified:

~~~sh
/path/to/ai run execute . \
  --task "..." \
  --runtime auto \
  --max-invocations 64 \
  --max-runtime 30m
~~~

Quota exhaustion is terminal evidence; it is not permission to make unbounded fallback calls.

## Memory

Use `--memory off` for no project-memory interaction.

When governed memory is enabled, remembered content remains untrusted evidence and cannot override task, permissions, safety or promotion rules.

See `docs/memory-governance.md` after the memory-governance release gate is enabled.

## Verify the checkout

The same core commands used by CI are:

~~~sh
go test ./...
go build ./...
go vet ./...
~~~

Linux CI additionally runs:

~~~sh
go test -race ./...
~~~

For runtime-specific setup and known limitations, see `COMPATIBILITY.md` and `TROUBLESHOOTING.md`.
