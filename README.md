# AI Engineering OS

Provider-independent multi-agent engineering platform for planning, executing, verifying, reviewing and safely promoting software changes.

Current release line: **0.1.0-rc.1**

AI Engineering OS combines deterministic orchestration with optional AI runtimes. The deterministic core remains authoritative for permissions, durable state, Quality, integrity, recovery and promotion.

## What is implemented

- specialized multi-agent task graphs and dependency-aware scheduling;
- persistent typed handoffs, messages and artifacts;
- read-only and isolated staged execution;
- Codex CLI and Cursor CLI runtime adapters;
- provider-independent model policy and bounded provider fallback;
- baseline-aware Quality with structured Go/Java/Jest/Vitest failures;
- deterministic OpenAPI and architecture compatibility gates;
- independent review and explicit promotion approval;
- durable run state, leases, pause/resume/retry/recovery;
- integrity-verified snapshots, archives and event compaction;
- bounded project memory with governed lifecycle;
- normalized external tool/MCP health;
- invocation/runtime budgets;
- local Control Plane API + embedded read-only web UI;
- Linux, Windows and macOS verification matrix.

## Safety model

AI output, repository content, external tool output and recalled memory are **untrusted evidence**.

They cannot bypass:

- declared filesystem scope;
- runtime/tool permissions;
- deterministic verification;
- independent review;
- original-tree integrity checks;
- contract/architecture gates;
- explicit promotion approval.

The original project is not used as the write workspace for staged mutation.

## Quick start

Build:

~~~sh
go build -o ai ./cmd/ai
~~~

Inspect the binary:

~~~sh
./ai version
./ai doctor .
~~~

Inspect a project:

~~~sh
./ai inspect . --write
~~~

Run a credential-free coordinated dry-run:

~~~sh
./ai run execute . --task "review the project architecture" --dry-run --memory off
~~~

Start the local Control Plane:

~~~sh
./ai run serve
~~~

Then open:

~~~text
http://127.0.0.1:8765/ui/
~~~

For the complete setup path see [docs/QUICKSTART.md](docs/QUICKSTART.md).

## Runtime support

Actual execution adapters currently exist for:

- **Codex CLI**
- **Cursor CLI**

Other tools/hosts may have registry, MCP or projection integration without being execution runtimes. See [docs/COMPATIBILITY.md](docs/COMPATIBILITY.md) for the evidence-based matrix.

## Documentation

- [Quickstart](docs/QUICKSTART.md)
- [Compatibility matrix](docs/COMPATIBILITY.md)
- [Troubleshooting](docs/TROUBLESHOOTING.md)
- [Migration guide](docs/MIGRATION.md)
- [Release readiness](docs/RELEASE_READINESS.md)
- [Threat model](docs/THREAT_MODEL.md)
- [Master plan](docs/MASTER_PLAN.md)
- [Runtime fallback](docs/runtime-fallback.md)
- [Run budgets](docs/run-budgets.md)
- [Promotion](docs/promotion.md)

## Verification

CI verifies:

- Linux;
- Windows;
- macOS.

Core matrix:

~~~sh
go test ./...
go build ./...
go vet ./...
go run ./cmd/ai doctor
go run ./cmd/ai version --json
~~~

Linux additionally runs:

~~~sh
go test -race ./...
~~~

## Release status

This repository uses release-readiness gates rather than treating merged PRs as production certification.

The 0.1.0 release-candidate line is intended to prove:

1. deterministic core and multi-agent coordination;
2. adversarial promotion/security boundaries;
3. multi-OS reproducibility;
4. coordinated E2E execution and promotion;
5. bounded provider usage;
6. reversible governed memory;
7. documented fresh-user operations.

See [docs/RELEASE_READINESS.md](docs/RELEASE_READINESS.md) for the current evidence ledger.

## Model policy

Do not hard-code a model per agent role.

The default remains provider-independent automatic selection, with explicit overrides only when a concrete runtime-specific requirement justifies them.
