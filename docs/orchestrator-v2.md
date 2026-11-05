# Orchestrator v2 — Execution Coordinator

## Purpose

The first Orchestrator v2 execution layer coordinates multiple specialist workers over the deterministic scheduler.

It does not hard-code AI providers or models.

A worker is an adapter that knows how to execute one assigned agent task.

~~~text
validated task DAG
      |
      v
deterministic scheduler
      |
      v
coordinator
  +---+---+---+
  |       |   |
worker  worker worker
  |       |   |
  +--- results/artifact refs
      |
      v
scheduler transitions + events
~~~

## Worker contract

A worker receives one already-authorized scheduler task.

It returns:

- concise result summary;
- artifact references;
- message references;
- error when the task failed.

The coordinator does not grant additional file scopes or permissions.

Provider-specific workers can later wrap Cursor, Codex, Claude Code or another runtime without changing the coordinator contract.

## Concurrency

The coordinator:

- uses the scheduler ready queue;
- respects a configured maximum concurrency;
- launches independent eligible workers concurrently;
- never launches tasks blocked by dependencies;
- inherits scheduler file-scope conflict protection.

The coordinator is therefore not allowed to bypass the DAG because a model wants to start work early.

## Failure behavior

A failed task is marked failed in the scheduler.

Downstream dependent tasks become blocked.

Independent branches may continue.

A missing worker for an assigned agent is an explicit task failure, not silent fallback to another role.

## Cancellation

Context cancellation:

- stops scheduling new tasks;
- cancels the scheduler's remaining non-terminal work;
- returns a cancelled run result.

Workers receive the same context and are expected to stop their provider/runtime work.

## Events

The coordinator emits typed lifecycle events:

- `run.started`
- `task.started`
- `task.completed`
- `task.failed`
- `run.completed`
- `run.cancelled`

The initial PR provides an in-memory event sink for tests and embedding.

Durable event persistence belongs to the Control Plane event-stream phase.

## Relationship to messages and artifacts

Workers may use the P6/P7 artifact and message layers and return their IDs.

The coordinator carries those references in task results and lifecycle events instead of copying full conversations.

## Current boundary

This is the first execution coordinator, not yet the full planning Orchestrator.

It does not yet:

- decompose a natural-language task into the DAG;
- dynamically replan;
- persist events;
- create isolated mutation workspaces per parallel worker;
- integrate/merge parallel staged outputs;
- automatically route inbox messages.

Those are later Orchestrator/Control Plane/merge-coordination steps.
