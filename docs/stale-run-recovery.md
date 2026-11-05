# Explicit Stale Run Recovery

## Purpose

A process crash can leave both durable runstate and the Control Plane saying `running` even though no healthy runner remains.

Recovery must never infer that condition from elapsed time alone.

The platform now requires durable lease evidence and an explicit human command.

## Command

~~~text
ai run recover <run-id> --stale-after 30s
~~~

Optional storage overrides:

~~~text
--state-dir
--events-dir
--control-dir
--checkpoint-dir
--json
~~~

The stale threshold must be at least five seconds.

## Preconditions

Recovery requires:

- durable run status = `running`;
- Control Plane status = `running`;
- an existing run lease;
- lease heartbeat older than the explicit stale threshold;
- durable work-unit/task identity consistent with Control Plane.

A live lease is never reclaimed.

A missing lease is not automatically treated as stale evidence.

## Stale lease claim

Recovery does not simply delete the old lease.

It atomically:

1. checks the lease is stale;
2. renames the stale lease out of the active lease path;
3. verifies owner/acquisition/heartbeat evidence did not change;
4. acquires a new temporary recovery lease with a different owner;
5. deletes the captured stale lease.

If the old runner wakes up, its next heartbeat observes the new owner and fails, cancelling its execution context.

There is no automatic takeover during normal execution.

## Read-only recovery

For a read-only run:

- completed tasks remain completed;
- queued/assigned/running/waiting tasks become queued;
- failed/blocked/cancelled evidence is preserved.

If there is no observed task failure, the run recovers to:

~~~text
paused
~~~

and can continue with `ai run resume`.

If a real task failure was already observed, the run recovers to:

~~~text
failed
~~~

so targeted retry can be used rather than hiding the failure.

## Mutation recovery

Mutation recovery requires an integrity-valid durable integration checkpoint.

Supported boundaries:

- `source-integrated`;
- `final-integrated`.

The checkpoint store is checked without restoration before stale lease reclaim.

At `source-integrated`:

- read-only/source tasks recover as completed;
- tests-only tasks recover as queued.

At `final-integrated`:

- all durable work units recover as completed.

The run becomes `paused` and can use checkpoint-based mutation resume.

No transient agent stage workspace is trusted.

## Atomic Control Plane reconciliation

Recovery appends one structured:

~~~text
run.recovered
~~~

event containing:

- recovered run status;
- checkpoint phase when applicable;
- normalized per-task statuses;
- stale duration evidence.

The projector applies the task/run reconciliation atomically.

For checkpoint recovery it also resets transient Quality state and restores the pipeline to the integrated checkpoint boundary.

## Durable state reconciliation

The durable run is updated before the recovery event is appended.

If Control Plane append fails, the command attempts to restore the original durable run record.

The temporary recovery lease is released only after state/event reconciliation completes.

## Safety boundary

Recovery never:

- executes an agent;
- restores a checkpoint into a writable workspace;
- applies source changes;
- promotes;
- commits;
- pushes.

It only moves an orphaned active run into a safe paused/failed state from which existing resume/retry controls may be used.
