# Integrity-Verified Control Plane Snapshots

## Purpose

Long Control Plane event histories should not require replaying every historical transition forever.

Snapshots are introduced as a **derived cache**, never as a replacement for the event log.

The event stream remains the durable audit source of truth.

## Snapshot contents

Each immutable snapshot records:

- schema version;
- run ID;
- event sequence covered by the snapshot;
- SHA-256 digest of the exact event-file prefix bytes;
- projected Control Plane state;
- creation timestamp;
- digest of the complete snapshot record.

Snapshots are stored outside the event run directory and are immutable per run+sequence.

## Creation rule

A snapshot can be created only after:

1. the complete event prefix is loaded through the normal EventStore path;
2. every event passes schema validation;
3. event filename/sequence continuity passes;
4. the full prefix successfully projects into Control Plane state;
5. the exact raw event bytes are hashed while the append lock is held.

Therefore a snapshot cannot be created from a structurally invalid event history.

## Prefix integrity

Snapshot verification recomputes the SHA-256 digest over the exact historical event files covered by the snapshot.

Verification fails when:

- a historical event is deleted;
- a sequence is duplicated or missing;
- a historical event file changes even if its JSON remains semantically valid;
- a historical entry becomes a symlink/non-regular file.

Appending newer events after the snapshot does not invalidate the historical prefix.

## Snapshot-record integrity

The snapshot record has its own digest.

Load rejects accidental/tampered changes to:

- projected state;
- prefix digest;
- sequence;
- metadata.

The projected state is additionally checked for internal consistency:

- Progress must match Tasks;
- Agent state must match Tasks;
- DAG edges must match task dependencies.

## Immutable history

Snapshots do **not** delete event files.

This first phase deliberately implements no destructive compaction.

Future projection acceleration may trust a snapshot only after its covered event prefix has been revalidated against the stored prefix digest.

## Filesystem rules

- snapshot root must be a regular directory;
- per-run snapshot directories must be regular;
- snapshot entries must be regular files;
- symlink substitution is rejected;
- duplicate snapshots for the same run+sequence are rejected.

## Next phase

Add snapshot-assisted projection:

1. load latest snapshot;
2. verify its historical prefix digest;
3. decode/project only suffix events after the snapshot;
4. verify the prefix again or hold a read/append lock across projection;
5. fall back to full replay when no valid snapshot exists.

Destructive event-log compaction remains out of scope until historical integrity can still be proven independently.
