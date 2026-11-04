# Integration Workspace Materialization

## Purpose

A validated multi-workspace integration plan can now be materialized into a fresh isolated integration workspace.

The original project remains read-only.

The integration workspace is the only place where outputs from multiple mutating task workspaces are combined.

## Preconditions

Materialization requires:

- a non-empty deterministic integration plan;
- every referenced task workspace;
- matching task/agent identity;
- exact declared file/action ownership;
- one shared pre-image digest.

A fresh sanitized view of the original project is created first.

If that fresh base no longer matches the plan's pre-image digest, materialization fails.

## Source freezing

Task inspection now records an immutable `ResultImage`.

The integration plan privately binds every create/modify operation to the exact file hash captured in that frozen image.

During materialization, copied bytes are hashed while reading.

If an agent modifies a source file after inspection, materialization fails even when:

- the same path is used;
- the same action is used.

A path inspected as deleted must still be absent. Recreating it after inspection also fails.

These hashes are internal safety evidence and are not exposed in the public integration-plan JSON.

## Applying the plan

The materializer:

1. creates a new sanitized integration workspace from the original project;
2. verifies its snapshot matches the common pre-image;
3. applies planned changes in deterministic plan order;
4. reads only regular non-symlink source files;
5. verifies create/modify source bytes against frozen inspection hashes;
6. applies deletes only when the inspected source remains absent;
7. snapshots the integrated result;
8. verifies the exact resulting create/modify/delete set equals the integration plan.

Any failure discards the temporary integration workspace.

## Original repository safety

Materialization never:

- writes to the original project;
- commits;
- pushes;
- promotes changes.

A final integration workspace can be discarded safely.

## Next quality boundary

The integrated workspace is the correct input for **run-wide deterministic verification and independent review**.

Only after that run-wide Quality Gate passes should the existing approval-gated promotion boundary be allowed to consider the combined change.
