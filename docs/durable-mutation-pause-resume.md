# Durable Mutation Pause and Resume

## Purpose

Coordinated write runs can now pause only at integrity-safe integration checkpoints and continue without replaying already integrated agent work.

## Mutation kinds

Durable work units record a mutation kind only after the task graph confirms the unit is actually mutating:

- `source`;
- `tests-only`;
- empty for read-only work.

The kind is derived from registry write capabilities, not agent names.

Legacy durable mutating units without a concrete kind remain readable, but mutation pause/resume rejects them.

## Pause request

`ai run pause <run-id>` can request pause for both read-only and checkpoint-capable mutation runs.

For mutation runs the request does **not** interrupt an active Stage/QA operation.

The runner continues until the next durable boundary.

## Safe mutation boundaries

### source-integrated

At this boundary:

- read-only pre-stage work is completed;
- source-writer tasks are completed;
- tests-only tasks remain pending;
- source stage workspaces have already been integrated;
- immutable source-integrated checkpoint exists.

A pause here discards transient source stage/integration workspaces after returning. Resume restores only the checkpoint.

### final-integrated

At this boundary:

- source work is integrated;
- tests-only QA is completed when present;
- all durable work units are completed;
- immutable final-integrated checkpoint exists;
- run-wide Quality has not started yet.

A pause here resumes directly into Quality/review/promotion.

## Resume

`ai run resume <run-id>` detects whether the paused run is read-only or mutating.

For mutating runs it requires:

- durable status = paused;
- Control Plane status = paused;
- concrete mutation kinds;
- latest projected checkpoint phase = source-integrated or final-integrated;
- valid immutable checkpoint;
- original project still matching checkpoint PreImage.

### Resume from source-integrated

The platform:

1. restores source integration into a fresh isolated workspace;
2. runs only tests-only QA units;
3. persists final-integrated checkpoint;
4. honors another pause request at that new boundary;
5. otherwise runs Quality/review/promotion.

Frontend/Backend/Database source stages are not replayed.

### Resume from final-integrated

The platform:

1. restores final integration into a fresh isolated workspace;
2. skips all source/QA staging;
3. runs baseline-aware Quality;
4. performs independent review/correction;
5. produces a persisted promotion candidate;
6. returns to the explicit approval boundary.

## Same run history

Mutation resume keeps the same run ID.

The existing Control Plane stream receives `run.resumed` with checkpoint phase metadata.

No task-created events are recreated.

QA lifecycle events are appended only when resume starts from source-integrated and QA still needs to run.

## Cancellation

The resumed mutation path installs the same durable cancellation watcher.

Cancellation during QA/Quality transitions the run to cancelled through the existing mutation abort boundary.

## Original project safety

Checkpoint restore always revalidates the original project against the immutable PreImage.

Resume never promotes automatically.

The final result still requires:

~~~text
ai run promote <run-id> --approve
~~~

before the original project is modified.
