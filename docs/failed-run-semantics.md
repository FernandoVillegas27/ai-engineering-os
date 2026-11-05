# Failed Run Semantics

## Purpose

Durable run state and Control Plane state must agree when a task failure makes a run terminal.

Previously the coordinator could finish scheduling a graph with failed/blocked tasks and still emit `run.completed`, while `runstate` correctly recorded the run as failed.

## Behavior

The coordinator now emits:

~~~text
run.failed
~~~

when the final scheduler progress contains failed or blocked tasks.

A clean graph still emits:

~~~text
run.completed
~~~

Cancellation remains:

~~~text
run.cancelled
~~~

## Control Plane

The projector now exposes a terminal `failed` run status and rejects new task execution events after that terminal state.

Artifacts/messages/approval audit events keep their existing post-terminal allowances.

## Mutation pipeline

Non-cancellation failures in the coordinated mutation pipeline also append a terminal `run.failed` event after persisting failed runstate.

This keeps read-only and write-capable runs semantically aligned.

## Why this matters

Targeted retry needs an unambiguous failed terminal source run.

A retry attempt can now reference a parent whose durable runstate and event-stream projection agree on failure.
