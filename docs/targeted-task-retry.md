# Targeted Read-Only Task Retry

## Purpose

A failed read-only coordinated run can retry one failed task without rebuilding the entire plan or modifying the failed parent run.

## Command

~~~text
ai run retry <run-id> <task-id>
~~~

Optional storage overrides:

~~~text
--state-dir
--events-dir
--artifacts-dir
--messages-dir
--control-dir
--json
~~~

## Preconditions

Retry requires:

- parent durable status = `failed`;
- parent Control Plane status = `failed`;
- durable intent present;
- target task status = `failed` or `blocked`;
- every target dependency completed in the parent;
- every parent work unit non-mutating.

If a dependency is not completed, retry fails and tells the caller to retry the dependency first.

## Mutation boundary

Runs containing any `mutating=true` work unit are rejected.

Source-writer and tests-only retries require persisted integration/stage checkpoints so the system can reconstruct the correct base. Retrying them without those checkpoints would risk integrating a new task result against stale outputs.

## Child-run model

Retry does not reopen the failed parent.

Instead it creates a new durable child run containing:

- `parent_run_id`;
- `retry_of_task_id`;
- original project/root;
- durable intent;
- same provider/model policy;
- one reconstructed read-only work unit.

The child task has no local dependencies because the parent dependencies are validated as completed before child creation.

## Collaboration context

The retried task reads its bounded inbox from the **parent run**.

The retry result is stored as a new handoff artifact owned by the **child run**.

This preserves source evidence while keeping attempts independently auditable.

## Control Plane

The child `run.started` event contains retry lineage.

Projected state and `GET /runs` expose:

- `parent_run_id`;
- `retry_of_task_id`.

The parent event stream and durable run record remain unchanged.

## Cancellation

The child run installs the normal durable cancellation watcher and can be cancelled independently through:

~~~text
ai run cancel <child-run-id>
~~~
