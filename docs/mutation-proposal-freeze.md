# Mutation Proposal Freeze

## Purpose

Before any specialist receives a write-capable staging runtime, its intended file ownership must be known and validated.

Proposal Freeze runs the existing proposal runtimes in read-only mode and freezes their declared file/action scopes.

## Flow

~~~text
read-only work units
  -> ProposalRuntime per agent
  -> task-alignment validation
  -> declared file/action validation
  -> frozen proposal set
  -> exact ownership conflict check
  -> only then stage
~~~

## Concurrency

Proposal generation may run concurrently with a bounded maximum.

Output ordering remains deterministic by work-unit ID.

A proposal failure cancels the remaining proposal context and the freeze operation fails closed.

## Frozen proposal

Each frozen item contains:

- task ID;
- agent ID;
- copied proposal;
- normalized declared `create|modify|delete` changes.

Returned proposal slices are copied so later mutation of a provider result cannot alter the frozen scope.

## Ownership rule

The initial multi-agent mutation policy rejects exact file ownership conflicts before staging.

Two proposals may not both declare the same repository-relative file.

This avoids spending write-capable model execution on a set that cannot be deterministically integrated under the current disjoint-file policy.

## Safety

Proposal generation remains read-only and uses the existing sanitized-provider runtimes.

No original project mutation occurs in this phase.

## Next

A frozen proposal can be passed to the existing Cursor/Codex StageRuntime.

The next adapter must:

- stage each frozen proposal into its own isolated workspace;
- verify returned task/agent/scope identity;
- freeze a post-stage ResultImage;
- reject any difference from the frozen declared ownership;
- convert successful stages into the isolated task-workspace/integration pipeline.
