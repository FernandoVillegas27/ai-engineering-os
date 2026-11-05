# Control Plane Recovery Control

## Purpose

A visual client should expose crash recovery only when the same deterministic evidence used by the CLI says recovery is safe.

The Control Plane run-controls response now includes explicit recovery capability.

## Response fields

`GET /runs/<run-id>/controls` can include:

~~~text
can_recover
recovery_to
recovery_from
~~~

Examples:

~~~text
can_recover=true
recovery_to=paused
recovery_from=durable-task-graph
~~~

or for mutation:

~~~text
can_recover=true
recovery_to=paused
recovery_from=source-integrated
~~~

## Evidence

Recovery is exposed only when:

1. durable runstate and projected Control Plane state agree;
2. both still say `running`;
3. the run lease heartbeat is stale past the configured threshold;
4. `DeriveRecovery` accepts the durable task state;
5. for mutation, the projected checkpoint is resumable and the checkpoint store validates it against the current original project.

A live or missing lease does not expose recovery.

An invalid mutation checkpoint does not expose recovery.

## Server configuration

`ai run serve` now accepts:

~~~text
--control-dir <path>
--recovery-stale-after <duration>
~~~

The default recovery stale threshold is 30 seconds.

The configured threshold must be at least five seconds.

## Safety

The endpoint remains read-only.

It does not claim a stale lease or mutate runstate.

It only tells the client whether the existing explicit command is currently safe:

~~~text
ai run recover <run-id>
~~~
