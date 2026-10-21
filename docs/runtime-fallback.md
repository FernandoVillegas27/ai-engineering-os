# Runtime fallback

AI Engineering OS can use more than one coding runtime for staged work.

## Runtime modes

`ai stage` supports:

- `--runtime auto` — default. Try Codex first and fall back to Cursor only when the provider is unavailable.
- `--runtime codex` — use Codex only.
- `--runtime cursor` — use Cursor CLI only.

Provider fallback is intentionally narrow. It activates for provider-availability failures such as exhausted usage/quota, authentication/login failures, missing provider CLI binaries, temporary network failures, and provider service unavailability.

Policy, safety, malformed-output, invalid-proposal, or unauthorized-file errors do **not** trigger fallback.

## Cursor CLI

Cursor is invoked through its `agent` CLI in non-interactive print mode.

Read-only proposal and review use Ask mode. Stage and correction use Agent mode against an isolated sanitized workspace.

The invocation requests Cursor sandboxing and an explicit workspace root. AI Engineering OS still performs its own deterministic post-run filesystem checks. Only approved safe file paths may remain changed.

The original repository is never the write working directory.

## Automatic flow

~~~text
proposal
  Codex
    ↓ provider unavailable
  Cursor

stage
  Codex
    ↓ provider unavailable
  Cursor

verification
  deterministic local verifier

review
  Codex
    ↓ provider unavailable
  Cursor

correction
  Codex
    ↓ provider unavailable
  Cursor
~~~

The fallback does not expand permissions. It changes only which provider performs an already-authorized operation.

## Authentication

AI Engineering OS does not forward provider API-key environment variables to Cursor. Cursor CLI is expected to use its normal local authenticated session.

If Cursor CLI is not installed or authenticated, `--runtime cursor` fails explicitly. In `auto` mode, if both providers are unavailable, the operation stops without touching the original repository.


## Platform behavior

Cursor's native sandbox is available on macOS and Linux. On those platforms AI Engineering OS invokes Cursor with `--sandbox enabled`.

On native Windows, Cursor does not provide that sandbox. AI Engineering OS therefore invokes Cursor with `--sandbox disabled`, which uses Cursor's non-sandbox execution controls, while retaining AI Engineering OS's own isolated sanitized workspace, declared-file approval boundary, and deterministic post-run filesystem validation.

This means Windows does not have the same OS-level command sandbox as macOS/Linux. The original repository is still not used as the write workspace, and undeclared filesystem changes inside the staged workspace are rejected.
