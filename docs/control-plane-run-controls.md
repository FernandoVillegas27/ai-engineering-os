# Control Plane Run Controls

## Purpose

A visual client must not guess which human actions are currently safe.

The Control Plane can now expose deterministic run controls derived from durable runstate, projected event state and checkpoint integrity.

## Endpoint

~~~text
GET /runs/<run-id>/controls
~~~

The response includes:

- `consistent`;
- `can_pause`;
- `can_resume`;
- `can_cancel`;
- `can_retry`;
- `pause_boundary`;
- `resume_from`;
- `retryable_task_ids`;
- optional reason when a control is unavailable.

## Consistency first

Controls are disabled when:

- runstate and Control Plane refer to different run IDs;
- durable and projected statuses disagree.

The UI never receives an optimistic action based on only one state source.

## Pause

Running read-only runs expose:

~~~text
can_pause=true
pause_boundary=task-boundary
~~~

Checkpoint-capable mutation runs expose:

~~~text
can_pause=true
pause_boundary=integration-checkpoint
~~~

Legacy mutation records without durable mutation kinds do not expose pause.

## Resume

Paused read-only runs can resume from their durable task graph.

Paused mutation runs can resume only when:

- mutation kinds are complete;
- projected checkpoint phase is resumable;
- the checkpoint store validates the checkpoint;
- the current original project still matches its PreImage.

The controls provider uses the read-only checkpoint `Check` operation; it does not restore or mutate a workspace.

## Retry

Failed read-only runs expose only failed/blocked tasks whose durable dependencies are all completed.

Mutation runs do not expose targeted retry yet because per-task stage outputs are not durable enough to reconstruct a single mutating task safely.

## Cancel

Running and paused runs expose cancellation.

Paused cancellation remains synchronous in the CLI because no active runner exists.

## Server wiring

`ai run serve` now opens:

- durable runstate;
- Control Plane event store;
- mutation checkpoint store;
- existing artifact/message/promotion stores.

Additional optional overrides:

~~~text
--state-dir <path>
--checkpoint-dir <path>
~~~

No HTTP mutation endpoint is introduced.

The endpoint only tells clients which existing CLI actions are safe.
