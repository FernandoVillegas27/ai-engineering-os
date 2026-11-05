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


## Snapshot-assisted projection

The Control Plane can continue projection from a validated snapshot instead of replaying its historical prefix.

The continuation path:

1. loads the latest immutable snapshot;
2. validates the snapshot record;
3. acquires the same append lock used by event writes;
4. recomputes the exact historical event-prefix digest;
5. decodes only event files with sequence greater than the snapshot boundary;
6. applies suffix events through the **same event-application function** used by full replay;
7. recomputes the historical prefix digest before returning;
8. fails closed if the prefix changed or any suffix event is invalid.

If no snapshot exists, projection falls back to normal full replay.

The base snapshot state is deep-cloned before suffix application, so a state read cannot mutate the persisted snapshot representation.

## HTTP behavior

When a snapshot store is configured, this endpoint uses verified snapshot projection:

~~~text
GET /runs/<run-id>/state
~~~

The event/audit surfaces remain event-log based:

~~~text
GET /runs/<run-id>/events
GET /runs/<run-id>/stream
~~~

A corrupt snapshot or changed historical event prefix returns an error. The server does not silently ignore a bad snapshot and fall back to a potentially different interpretation.

## Creating snapshots

Snapshot creation is explicit:

~~~text
ai run snapshot <run-id>
~~~

Optional overrides:

~~~text
--events-dir <path>
--snapshot-dir <path>
--json
~~~

Storage resolution for snapshots is:

1. explicit `--snapshot-dir`;
2. `AI_ENGINEERING_OS_SNAPSHOT_DIR`;
3. user config `ai-engineering-os/control-plane-snapshots`.

`ai run serve` accepts the same `--snapshot-dir` override.

A read-only GET never creates or rewrites a snapshot.

## Compaction boundary remains unchanged

Snapshot-assisted projection reduces historical **decode/projection** work, but historical event files remain present.

No destructive compaction is enabled yet.

Deleting historical event files would remove the evidence required to recompute the snapshot prefix digest, so any future compaction format must preserve independently verifiable historical integrity before deletion is permitted.
