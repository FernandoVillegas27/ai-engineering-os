# Durable Read-Only Pause and Resume

## Purpose

Long coordinated read-only runs can now stop at a safe task boundary and continue later without reclassifying or rebuilding the plan from chat context.

## Commands

Request a graceful pause:

~~~text
ai run pause <run-id>
~~~

Resume the same durable run:

~~~text
ai run resume <run-id>
~~~

Optional storage overrides remain available for state, events, artifacts, messages and run control.

Resume also accepts:

~~~text
--max-concurrency <1..16>
~~~

## Graceful pause semantics

Pause is not process suspension.

When a durable pause request appears:

1. the coordinator stops acquiring new tasks;
2. currently running tasks are allowed to finish;
3. their artifacts/messages are persisted normally;
4. once no task is active, the coordinator emits `run.paused`;
5. durable runstate transitions from `running` to `paused`.

This prevents partially executed agent work from being treated as a checkpoint.

If all work finishes before the pause boundary, the run completes normally instead of becoming paused.

If active work fails before the pause boundary, the run fails rather than hiding the failure behind a paused state.

## Durable pause request

Pause requests are immutable/idempotent per-run control records.

The Control Plane distinguishes:

~~~text
run.pause-requested
run.paused
run.resumed
~~~

Projected state exposes `pause_requested`.

The run index also exposes pending pause/cancel flags for visual clients.

## Resume reconstruction

Resume requires:

- durable run status = `paused`;
- Control Plane status = `paused`;
- durable intent;
- non-mutating work units only;
- no running/failed/blocked/cancelled task left in the paused checkpoint.

The platform reconstructs:

- work units;
- permissions;
- capabilities/skills/tools;
- provider/model policy;
- dependency graph;
- completed tasks;
- queued/waiting tasks.

Completed tasks initialize the scheduler as completed and never rerun.

Other safe pending tasks are reconstructed as queued and the scheduler derives readiness from persisted dependencies.

No classifier or planner is invoked during resume.

## Event-stream continuity

Resume keeps the same `run-id`.

The Control Plane appends:

~~~text
run.resumed
~~~

to the existing stream.

It does **not** recreate `task.created` or `task.assigned` events.

New task execution/artifacts/messages continue on the original run history.

## Provider routing

Resume reuses the durable runtime selection:

- dry-run;
- Codex;
- Cursor;
- Auto fallback where supported.

No model is tied permanently to an agent role.

## Cancellation while paused

A paused run has no active process that can observe a cancel marker.

Therefore:

~~~text
ai run cancel <paused-run-id>
~~~

performs a synchronous durable transition:

~~~text
paused -> cancelled
~~~

and appends cancellation audit events while clearing the pause marker.

## Safety boundary

Pause/resume currently rejects any run containing `mutating=true` work units.

Mutation runs require persisted stage/integration checkpoints before they can be resumed without risking stale or incompatible workspace state.

This phase intentionally does not fake that capability.
