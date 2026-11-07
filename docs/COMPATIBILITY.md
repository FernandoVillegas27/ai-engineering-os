# Compatibility matrix

This document describes **implemented and verified** support. Roadmap goals and registry declarations are not presented as runtime support.

## Operating systems

| Platform | Core CI | Go tests/build/vet | Doctor smoke | Race tests | Notes |
| --- | --- | --- | --- | --- | --- |
| Linux | Yes | Yes | Yes | Yes | Cursor native sandbox can be enabled |
| Windows | Yes | Yes | Yes | No | Symlink creation may require privileges; Cursor lacks the same native sandbox |
| macOS | Yes | Yes | Yes | No | Cursor native sandbox can be enabled |

The GitHub Actions matrix uses `ubuntu-latest`, `windows-latest` and `macos-latest`.

## Execution runtimes

| Runtime | Adapter exists | Read-only analysis | Staged mutation / review path | Provider fallback |
| --- | --- | --- | --- | --- |
| Codex CLI | Yes | Yes | Yes | Primary in `auto` |
| Cursor CLI | Yes | Yes | Yes | Fallback in `auto` for provider-availability failures |
| Claude Code | No execution adapter | No | No | No |
| OpenCode | No execution adapter | No | No | No |
| VS Code Copilot | No execution adapter | No | No | No |
| IntelliJ | No execution adapter | No | No | No |

Engram and Gentle-AI may know how to project/configure several agent hosts. That does **not** mean AI Engineering OS currently has an execution runtime adapter for every one of those hosts.

## IDE integration status

- Cursor: actual CLI runtime adapter exists.
- VS Code: no dedicated AI Engineering OS extension/runtime adapter yet.
- IntelliJ IDEA: no dedicated AI Engineering OS plugin/runtime adapter yet.
- The CLI, project metadata and local Control Plane can still be used alongside any IDE.

Dedicated IDE clients remain adoption work, not a prerequisite for deterministic core operation.

## External integrations

| Integration | Implemented adapter | Role | Required for core |
| --- | --- | --- | --- |
| Engram | Yes | optional memory mirror / MCP configuration | No |
| Context7 | Yes | documentation provider / MCP launcher health | No |
| Gentle-AI | Yes | runtime projection / doctor integration | No |
| GitHub | Registry/host connector contract | repository operations when connected | No |
| Codegraph | Registry MCP declaration | code intelligence when host provides it | No |
| Playwright | Registry MCP declaration | browser testing when host provides it | No |

A registry declaration is a capability contract, not proof that a local MCP server is installed and healthy.

## Verification toolchains

Current deterministic local verification discovers and runs relevant project tooling.

Structured failure identity is strongest for:

- Go `go test -json`;
- Java Gradle/JUnit patterns;
- Java Maven/Surefire patterns;
- Java compile errors;
- Jest JSON reporter;
- Vitest JSON reporter.

Other detected test commands may still execute without a complete structured failure parser. In particular, Angular `ng test` is intentionally not reported as structurally complete unless the configured test builder can be identified safely.

## Model policy

No agent role is pinned to a provider-specific model.

Supported policy concepts remain provider-independent:

- auto;
- economy;
- balanced;
- deep;
- explicit model override when the selected runtime supports it.

The default policy should remain `auto` for normal usage.

## Control Plane

The HTTP API and embedded UI are local operational surfaces backed by durable Control Plane state/events.

The web UI is read-only. It does not grant mutation or promotion capability.

## Release-candidate scope

The `0.1.0-rc.1` line targets:

- deterministic planning;
- durable coordinated runs;
- Codex/Cursor runtime execution;
- isolated staged mutation;
- Quality/review/promotion gates;
- recovery, snapshots and compaction;
- bounded provider usage;
- governed project memory;
- local Control Plane API/UI;
- Linux/Windows/macOS core verification.

Items outside that scope must remain documented as roadmap work rather than implied support.


## Repository-owned verification code

Mutating coordinated runs are supported only when the operator explicitly supplies `--trust-project-code`.

Reason: test/build commands are executable repository content. AI Engineering OS isolates their user/cache environment and enforces post-verification workspace/promotion checks, but it does not claim a universal cross-platform filesystem/network sandbox for arbitrary third-party test programs.

Therefore:

- read-only/dry-run operation remains available for untrusted repositories;
- unattended mutating verification of an untrusted repository is outside the release scope;
- trusting project code is an explicit human decision and is persisted durably;
- that decision does not expand agent/model permissions.
