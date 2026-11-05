# Durable Mutation Checkpoints

## Purpose

Mutating runs cannot be resumed safely from transient per-agent stage directories.

The platform now persists immutable integrated-workspace checkpoints at phases where all prior temporary stage outputs have already been combined and validated.

## Initial phases

Two checkpoint phases are defined:

~~~text
source-integrated
final-integrated
~~~

### source-integrated

Created immediately after Frontend/Backend/Database source outputs are integrated into one isolated workspace.

At this point source-agent stage workspaces are no longer needed to reconstruct the combined implementation.

### final-integrated

Created after the tests-only QA phase.

It represents the exact workspace that enters the run-wide Quality Gate:

- source integration;
- plus QA test overlay when applicable.

## Stored evidence

Each checkpoint contains:

- run ID;
- phase;
- original project root;
- full immutable PreImage snapshot;
- PreImage digest;
- integrated workspace digest;
- exact relative create/modify/delete set;
- creation timestamp;
- a private copy of the integrated workspace.

The checkpoint store is outside the original project.

## Save-time validation

Before a checkpoint is committed, the store verifies:

1. integrated workspace exists and is regular;
2. current integrated filesystem changes exactly match the declared change set;
3. the original project still matches the integrated workspace PreImage;
4. the copied checkpoint workspace has the exact same digest.

A checkpoint is immutable for a given run+phase.

## Restore-time validation

Restore:

1. validates the manifest schema and identity;
2. reconstructs and validates the PreImage;
3. rechecks the current original project against that PreImage;
4. verifies the persisted checkpoint workspace digest;
5. verifies the persisted change set;
6. copies the checkpoint into a fresh isolated integration workspace;
7. verifies the fresh restored digest again.

The durable checkpoint itself is never reused as a writable execution workspace.

## Filesystem safety

Checkpoint roots, run directories, phase directories, manifests and source workspaces reject symlink substitution where relevant.

Checkpoint IDs and phases are constrained to safe known values.

## Control Plane

Successful checkpoint creation emits:

~~~text
checkpoint.created
~~~

with the phase in structured event data.

Projected pipeline state exposes the latest durable `checkpoint_phase`.

## CLI storage

Coordinated mutating runs accept:

~~~text
--checkpoint-dir <path>
~~~

Storage resolution follows:

1. explicit CLI override;
2. `AI_ENGINEERING_OS_CHECKPOINT_DIR`;
3. user config under `ai-engineering-os/run-checkpoints`.

## Next

These checkpoints are the prerequisite for mutation-aware pause/resume and targeted retry.

A future mutating resume should restore the latest valid checkpoint and continue from the **next phase**, never replay already integrated source stages blindly.
