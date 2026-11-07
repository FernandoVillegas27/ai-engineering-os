# Local Control Plane observability

AI Engineering OS exposes aggregate observability from the same durable Control Plane used by the CLI and IDE clients.

There is **no outbound telemetry by default**.

## Endpoints

When the loopback server is running:

~~~text
GET /observability
GET /metrics
~~~

Both endpoints are read-only and inherit the Control Plane security headers.

### JSON

`/observability` returns aggregate current state including:

- run counts by status;
- durable event count;
- task counts by status;
- projected agent count;
- durable message/artifact totals;
- pending approval count;
- runs with pause/cancel requests;
- newest event timestamp.

### Prometheus

`/metrics` exposes the same aggregate information in Prometheus text format.

Examples:

~~~text
ai_engineering_os_runs{status="running"} 1
ai_engineering_os_tasks{status="failed"} 0
ai_engineering_os_events_total 185
ai_engineering_os_pending_approvals 1
~~~

## Privacy boundary

The metrics intentionally do **not** include labels or fields containing:

- run IDs;
- task IDs;
- agent IDs;
- project names;
- prompts;
- summaries;
- source paths;
- artifact/message bodies;
- model/provider output;
- secrets.

The endpoint derives metrics from durable local state and does not persist a second telemetry database.

## Network boundary

`ai run serve` remains loopback-only.

The observability endpoints do not:

- push to a remote Prometheus service;
- contact OpenTelemetry collectors;
- phone home;
- enable analytics;
- create network permissions for agents.

An operator may choose to scrape the local endpoint with their own tooling, but that is outside automatic AI Engineering OS behavior.

## Dashboard

The embedded dashboard uses `/observability` to show a small aggregate events/tasks/approvals summary in the runs sidebar.

Per-run detail continues to come from the existing run/state/event/message/artifact APIs and SSE stream.

## Failure behavior

Observability is derived from verified Control Plane state.

If durable run history is corrupt or cannot be projected, observability fails closed instead of silently reporting misleading metrics.
