# Durable Run Cancellation

## Purpose

An active coordinated run can now receive a cancellation request from another process/terminal without directly mutating its in-memory scheduler state.

## Command

~~~text
ai run cancel <run-id>
~~~

Optional storage overrides:

~~~text
--control-dir <path>
--state-dir <path>
--events-dir <path>
--json
~~~

## Durable control request

Cancellation is persisted as an immutable per-run control record.

Repeated requests are idempotent.

Storage resolves through:

1. explicit `--control-dir`;
2. `AI_ENGINEERING_OS_CONTROL_DIR`;
3. user config under `ai-engineering-os/run-control`.

Unsafe IDs and symlink control entries are rejected.

## Active-run watcher

Read-only coordinated runs and coordinated mutation runs install a cancellation watcher after transitioning to `running`.

The watcher polls the durable control store and cancels the run context when a request appears.

Provider/runtime calls receive that same context through the existing execution/proposal/stage/quality boundaries.

## State semantics

Requesting cancellation emits:

~~~text
run.cancel-requested
~~~

The Control Plane keeps the run status as `running` while exposing:

~~~text
cancel_requested: true
~~~

Once the active process acknowledges the cancellation, the run transitions to `cancelled` and emits:

~~~text
run.cancelled
~~~

At that point `cancel_requested` returns to false because cancellation is no longer pending.

## Mutation runs

A cancellation-induced error is not recorded as a generic failure.

Non-completed work units become `cancelled`, the durable run becomes `cancelled`, and the Control Plane receives `run.cancelled`.

Ordinary non-cancellation errors still produce a failed durable run.

## Limitations

This phase adds cancellation only.

Pause/resume/retry require additional durable execution checkpoints and are intentionally separate.

A runtime/provider that ignores context cancellation may not stop until its current subprocess boundary returns; the coordinator will not schedule additional work after cancellation.


## Event-stream binding

Before a cancel marker is persisted, the CLI validates that the selected Control Plane store already contains the target run and starts with `run.started`.

This prevents a mismatched `--events-dir` from:

- creating a detached cancellation event stream;
- persisting a cancellation signal for a command that ultimately reports configuration failure.

Validation happens before the durable control request is written.
