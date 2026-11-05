# Durable Run Lease and Heartbeat

## Purpose

A durable run can be controlled or resumed from multiple processes, so the platform needs a deterministic answer to:

> Which process currently owns active execution for this run?

A run lease prevents concurrent active runners and provides heartbeat evidence for later crash recovery.

## Lease record

Each active run owns one immutable-identity lease record containing:

- schema version;
- run ID;
- random owner ID;
- acquisition timestamp;
- latest heartbeat timestamp.

Lease files live in the existing run-control store.

## Acquisition

Lease acquisition uses exclusive file creation.

If any lease already exists for the run, acquisition fails.

This is true even when the existing heartbeat appears stale.

There is **no automatic stale takeover** in this phase.

A later explicit recovery command may reclaim a stale lease only after validating durable run/checkpoint state.

## Heartbeat

`StartLeaseSession` renews the heartbeat periodically.

The in-memory lease identity remains immutable.

If heartbeat renewal fails because the lease disappears, changes owner or becomes invalid:

1. the lease session records an error;
2. the active execution context is cancelled;
3. the runner stops scheduling/continuing work through the existing context boundaries.

This prevents a process from continuing execution after losing durable ownership.

## Release

Normal runner shutdown releases the lease only when the caller still owns the same owner ID.

Owner mismatch fails closed.

## Stale detection

`LeaseStale` compares the current time with the most recent heartbeat, not the acquisition timestamp.

It requires an explicit positive stale threshold.

A future heartbeat is rejected as invalid clock/order evidence.

## Active runner integration

The following active execution paths now acquire a lease:

- coordinated read-only execute;
- coordinated mutation execute;
- targeted read-only retry child run;
- read-only same-run resume;
- mutation checkpoint resume.

For same-run resume, lease acquisition happens **before** the durable transition from `paused` to `running`.

Therefore concurrent resume attempts cannot both enter execution.

## Cancellation

The lease session wraps the existing durable cancellation watcher.

The active context is cancelled by either:

- user cancellation request;
- lease ownership/heartbeat failure;
- parent context cancellation.

## Recovery boundary

This PR intentionally does not delete or reclaim stale leases automatically.

Crash recovery requires a separate explicit command that can evaluate:

- durable run status;
- Control Plane status;
- lease heartbeat age;
- task graph/checkpoint availability;
- original project integrity.

Only then may a stale lease be reclaimed.
