# Long-Run Recovery and History Soak Coverage

## Purpose

The durable execution stack must remain correct under scheduler jitter, repeated heartbeats, large task sets, and growing Control Plane histories.

This coverage exercises those boundaries without introducing time-based correctness assumptions into production behavior.

## Lease lifecycle soak

Lease session tests now wait for observable heartbeat advancement with a bounded deadline instead of assuming a heartbeat will always occur inside one fixed sleep window.

The soak lifecycle repeatedly verifies:

- exclusive lease acquisition;
- multiple heartbeat advances;
- stable owner identity;
- explicit stop;
- lease release.

This removes CI sensitivity to scheduler pauses while keeping the production heartbeat semantics unchanged.

## Large recovery derivation

Recovery derivation is exercised with hundreds of durable work units and Control Plane task states.

The tests verify that:

- completed work remains completed;
- queued/assigned/running/waiting work returns to queued;
- one observed failed task preserves failure evidence and makes the recovered run failed;
- task identity remains complete across the full run.

## Long Control Plane history

The Control Plane soak test grows one run through multiple generations:

~~~text
40 events
  -> snapshot -> archive -> compact
80 events
  -> snapshot -> archive -> compact
120 events
  -> snapshot -> archive -> compact
160 events
  -> snapshot -> archive -> compact
+ 25 live suffix events
~~~

It then verifies:

- every historical sequence remains reconstructable;
- all earlier snapshots still verify after later compaction generations;
- new appends continue at the exact next sequence;
- the final full replay equals verified snapshot+suffix projection;
- artifact counts and final sequence remain exact.

These tests are skipped only under `go test -short`. The normal CI suite runs them.
