# Controlled Stage Execution

## Purpose

Controlled staging is the first write-capable layer in AI Engineering OS.

It does **not** write to the original repository.

The workflow is:

~~~text
Task
  ↓
Classify + Plan
  ↓
Codex proposal in sanitized read-only view
  ↓
Validate proposal/patch paths
  ↓
Show exact patch
  ↓
Explicit APPLY confirmation
  ↓
Create isolated sibling workspace
  ↓
git apply --check
  ↓
git apply
  ↓
Verify file postconditions
  ↓
Keep isolated workspace for inspection
~~~

## Command

~~~powershell
go run ./cmd/ai stage . --task "..." --runtime codex
~~~

The command prints the exact generated proposal and waits for:

~~~text
APPLY
~~~

Anything else cancels the operation.

## Safety boundary

Controlled staging v1:

- requires exactly one Work Unit;
- keeps the original Work Unit permission envelope read-only;
- generates the proposal from the sanitized safe view;
- validates every patch path against the same positive file policy;
- rejects absolute paths and traversal;
- rejects sensitive paths;
- rejects unsupported file types;
- rejects undeclared patch files;
- rejects proposal/patch action mismatches;
- rejects renames in v1;
- runs `git apply --check` before applying;
- applies only inside a sibling isolated workspace;
- does not commit;
- does not push;
- does not modify the original repository;
- does not run tests yet.

## Patch actions

v1 supports:

- create;
- modify;
- delete.

Renames are deliberately rejected until rename semantics have an explicit safety contract.

## Isolated workspace

A successful staging operation preserves the generated workspace so the user can inspect it.

Example:

~~~text
C:\work\
├─ project\
└─ .ai-engineering-os-stage-12345\
~~~

The original project remains unchanged.

The retained stage workspace is intentionally temporary operational state. A later lifecycle command will manage cleanup and promotion.

## Git subprocess

The local `git apply` subprocess receives a minimal environment and disables:

~~~text
GIT_CONFIG_NOSYSTEM=1
GIT_TERMINAL_PROMPT=0
~~~

No network operation is performed.

## Next stages

Controlled staging is not promotion.

Future steps will add:

1. verification/tests inside the isolated workspace;
2. independent review of the resulting diff;
3. explicit promotion to the original working tree;
4. separate commit/push approval gates.
