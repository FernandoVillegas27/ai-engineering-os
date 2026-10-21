# Quality Loop

The Quality Loop runs automatically after a controlled stage succeeds.

## Flow

~~~text
isolated staged workspace
  ↓
deterministic verification
  ↓
if verification fails → bounded correction
  ↓
verification again
  ↓
independent read-only review
  ↓
if review rejects → bounded correction
  ↓
verification + review again
  ↓
PASS or stop after max attempts
~~~

The default maximum is three quality attempts.

## Verification

The local verifier detects supported project types from the isolated workspace and runs deterministic checks:

- Go: `go test ./...`
- Maven: `mvn -q test`
- Gradle: `gradle test --no-daemon`
- Python with `pyproject.toml`: `python -m pytest`
- Node projects with a real `package.json` test script: `npm test`, with non-watch flags for Vitest, Jest, and Angular test scripts.

Verification runs only in the isolated staged workspace. Environment forwarding is allowlisted and arbitrary secret variables are not forwarded.

A missing required verification executable is treated as a verification failure/infrastructure blocker; it is not evidence that the original repository changed.

## Independent review

After deterministic verification passes (or no supported deterministic verifier is detected), a separate Codex read-only review runs against a fresh sanitized view of the staged workspace.

Medium, high, or critical findings reject the quality gate. Info and low findings are non-blocking.

The reviewer is logically independent from the implementation runtime and receives the original task plus the approved file scope.

## Correction loop

When verification or review rejects the staged implementation, the Codex correction runtime may modify the existing isolated workspace.

Corrections are constrained to the file paths that were explicitly approved in the original proposal.

After every correction:

1. deterministic verification runs again;
2. independent review runs after verification succeeds;
3. the loop stops immediately when both gates pass;
4. the loop stops after the configured maximum number of attempts.

If Codex changes an undeclared file, creates a sensitive/unsupported entry, or makes no corrective change, the loop fails closed.

## Safety boundary

The Quality Loop never promotes files back to the original repository.

It does not commit or push.

The original repository remains unchanged while verification, review, and correction operate on the isolated staged workspace.

Promotion is a separate future boundary and requires its own explicit approval.
