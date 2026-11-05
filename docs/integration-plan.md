# Multi-Workspace Integration Planning

## Purpose

Multiple mutating agents must not copy their outputs directly into the original project or into each other's workspaces.

This phase introduces a deterministic, read-only integration plan over successfully inspected isolated task workspaces.

It does not copy or apply files.

## Inputs

The planner receives:

- task workspaces produced by the isolated workspace manager;
- one inspection result per task workspace.

Every workspace must contain:

- stable task ID;
- agent ID;
- immutable pre-image;
- declared exact file/action scope.

## Invariants

The planner rejects the integration when:

- no workspaces are provided;
- inspections are missing;
- an inspection references an unknown task;
- task IDs are duplicated;
- any workspace lacks a pre-image;
- workspaces do not share the same pre-image digest;
- declared file ownership conflicts across tasks;
- an inspected change is outside its declared scope;
- an inspected action differs from its declaration;
- no actual changes remain.

## Output

The plan contains only:

- common pre-image digest;
- task ID;
- agent ID;
- repository-relative path;
- action.

It deliberately does not expose absolute task-workspace paths.

Changes are ordered deterministically by relative path, task ID, then action.

## Why a common pre-image matters

All parallel mutating tasks must start from the same sanitized project state.

If one task was prepared from a different original snapshot, integration is rejected instead of attempting an AI-assisted merge over incompatible bases.

## Initial conflict policy

The first integration policy is deliberately strict:

> exact declared file ownership must be disjoint across task workspaces.

No same-file concurrent edits are merged automatically.

Future conflict resolution can be introduced only with explicit base/ours/theirs evidence and run-wide re-verification.

## Next phase

A later materializer can consume this plan and create a fresh **integration workspace** from the same base, then copy each planned change into that isolated tree.

That integration workspace must pass run-wide verification/review before the existing promotion boundary can be considered.
