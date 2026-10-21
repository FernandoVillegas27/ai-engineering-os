# Controlled Stage Execution

## Purpose

Controlled staging is the first write-capable layer in AI Engineering OS.

It never writes to the original repository.

The workflow is:

~~~text
Task
  ↓
Classify + Plan
  ↓
Codex proposal in sanitized read-only view
  ↓
Validate declared file paths/actions
  ↓
Show proposal
  ↓
Explicit APPLY confirmation
  ↓
Create isolated sanitized sibling workspace
  ↓
Codex runs with workspace-write ONLY inside that workspace
  ↓
Deterministically compare before/after files
  ↓
Reject undeclared, sensitive or unsupported changes
  ↓
Keep isolated workspace for verification
~~~

## Why stage does not apply AI-generated patches

`ai propose` may still return a unified diff for human inspection.

`ai stage` deliberately does **not** use that diff as its write mechanism. Model-generated patch syntax can be malformed even when the intended source change is correct.

Instead, after explicit approval, the Codex stage runtime edits an isolated sanitized workspace directly. AI Engineering OS then computes the actual changed files from the filesystem.

This removes model-generated hunk counts and patch formatting from the trusted write path.

## Command

~~~powershell
go run ./cmd/ai stage . --task "..." --runtime codex
~~~

The command shows the proposal and waits for:

~~~text
APPLY
~~~

Anything else cancels the operation.

## Safety boundary

Controlled staging:

- requires exactly one Work Unit in this version;
- keeps the original project Work Unit read-only;
- generates the proposal from a sanitized safe view;
- validates all declared file paths against the positive file policy;
- creates a new sanitized isolated sibling workspace after approval;
- runs Codex with `--sandbox workspace-write` in that isolated workspace only;
- prohibits commit, push, dependency installation, secrets, production and network use in the runtime instruction;
- snapshots the sanitized workspace before execution;
- computes real file changes after execution;
- rejects sensitive, unsupported, symlink or non-regular files;
- rejects changes to files not declared in the approved proposal;
- rejects action mismatches such as declaring modify but actually creating/deleting;
- allows the runtime to touch fewer files than proposed, but never additional files;
- does not commit;
- does not push;
- does not modify the original repository;
- does not run verification tests yet.

## Isolated workspace

A successful stage operation preserves the workspace for the next verification phase.

Example:

~~~text
C:\work\
├─ project\
└─ .ai-engineering-os-stage-12345\
~~~

The original project remains unchanged.

## Next stage

The retained workspace becomes the input for:

1. automated tests and verification;
2. independent review;
3. controlled correction loops;
4. eventual explicit promotion to the original working tree.
