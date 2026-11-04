# Multi-Agent Control Plane

## Purpose

The Control Plane is the observable backend state of a multi-agent engineering run.

It is the functional layer behind any future dashboard, terminal view, IDE panel or 3D visualization.

The UI is replaceable. The event/state semantics are the product.

## Event stream

Every important run transition is represented as a typed, versioned event with:

- stable event ID;
- monotonically increasing per-run sequence;
- run ID;
- event type;
- optional task and agent IDs;
- concise summary;
- artifact references;
- message references;
- structured JSON data;
- timestamp.

Initial event families include:

- run lifecycle;
- task lifecycle;
- agent messages;
- artifacts;
- verification;
- baseline comparison;
- correction;
- review;
- approvals;
- runtime/model routing;
- tool execution.

## Durable ordering

Events are persisted per run.

Append operations use an exclusive per-run lock and allocate a contiguous sequence number.

Replay fails closed on:

- sequence gaps;
- duplicate/out-of-order sequence;
- malformed JSON;
- unsupported event types/schema versions;
- symlink/non-regular event entries;
- mixed run IDs.

This makes state reconstruction deterministic.

## State projector

The Control Plane can rebuild a run snapshot entirely from its event stream.

The projected state contains:

- run status;
- last sequence;
- task states;
- agent states;
- deterministic task progress counts;
- verification/review status;
- baseline comparison count;
- correction count;
- message count;
- artifact count;
- pending approval count;
- selected runtime/model summary.

Agent state is derived from real task state, not from a model-generated percentage.

## Coordinator integration

The Orchestrator v2 coordinator can use a `CoordinatorSink` that persists its lifecycle events directly into the Control Plane.

Therefore an executed DAG can already be reconstructed after the coordinator process exits.

Example conceptual projection:

~~~text
run: running

agents:
  architect: completed
  backend-engineer: running
  frontend-engineer: waiting
  qa-engineer: waiting

tasks:
  architecture: completed
  backend: running
  frontend: waiting
  qa: waiting

quality:
  verification: running
  baseline comparisons: 1

messages: 6
artifacts: 4
pending approvals: 1
~~~

## Future clients

The same projected state can back:

- `ai run status`;
- `ai run graph`;
- `ai run agents`;
- `ai run messages`;
- VS Code / IntelliJ panels;
- web dashboards;
- a Promix-like 2D/3D visualization.

No visualization should need to understand provider conversations.

## Current boundary

This phase provides:

- durable event protocol;
- ordered local event store;
- deterministic state replay;
- coordinator lifecycle integration.

It does not yet provide:

- WebSocket/SSE live streaming;
- a web server;
- persistent event-store compaction/snapshots;
- stale-lock recovery after process crashes;
- UI;
- dynamic orchestrator replanning.

Those can be added without changing the principle that the event stream is authoritative.


## CLI inspection

The first human-facing Control Plane client is the CLI.

~~~text
ai run control <run-id>
ai run events <run-id>
~~~

Use `--json` for machine-readable output.

`ai run control` replays the authoritative event stream and prints:

- run status and last event sequence;
- deterministic task-state progress;
- agents and their current derived state;
- active tasks per agent;
- task ownership/status;
- verification and review state;
- baseline/correction counters;
- message/artifact counts;
- pending approvals;
- runtime/model summary when present.

`ai run events` prints the ordered event history for audit/debugging.

Event storage resolution order is:

1. explicit `--events-dir`;
2. `AI_ENGINEERING_OS_EVENTS_DIR`;
3. the OS user configuration directory under `ai-engineering-os/events`.

This terminal view is deliberately a client of the same Control Plane state that a future web/IDE/3D interface will consume.


## Live read-only API

The first live client transport is a local HTTP/SSE server:

~~~text
ai run serve
~~~

Default address:

~~~text
127.0.0.1:8765
~~~

The first version rejects non-loopback bind addresses.

Endpoints:

~~~text
GET /health
GET /runs/<run-id>/state
GET /runs/<run-id>/events
GET /runs/<run-id>/stream?after=<sequence>
~~~

### State

`/state` returns the deterministic projection of the authoritative event stream.

### Events

`/events` returns the ordered persisted event list.

### SSE stream

`/stream` emits Control Plane events as Server-Sent Events.

Clients can resume from a known sequence with:

~~~text
?after=42
~~~

Only events with sequence greater than 42 are emitted.

This allows a web/IDE/3D client to reconnect without replaying the entire event history.

### Security boundary

The initial server:

- is read-only;
- accepts only GET for run resources;
- binds only to localhost/loopback;
- exposes no mutation endpoint;
- sets defensive browser headers;
- adds no permissive CORS headers;
- reads the same event store used by CLI inspection.

Remote access/authentication is intentionally not supported yet.

A future UI should either run from the same trusted local application boundary or introduce explicit authentication before any non-loopback exposure.


## Task graph metadata

The Control Plane state now preserves the scheduler graph itself.

Each task projection can include:

- `depends_on`;
- `mutating`;
- repository-relative `scopes`.

The projected state also exposes deterministic graph edges:

~~~json
{
  "edges": [
    {"from": "work-01-architect", "to": "work-02-backend"}
  ]
}
~~~

This lets visual clients render the real task/agent DAG and highlight mutation-scope conflicts without reconstructing scheduling rules from prose or model output.

The graph is sourced from the scheduler tasks recorded at run initialization.


## Event-store append-lock crash recovery

Control Plane event append uses the same ownership-aware filesystem lock as the message bus.

A process crash can therefore leave an old lock file without permanently freezing event ingestion. Stale recovery is age-bounded, symlink-safe, cancellation-aware, and release checks the ownership token before removing the lock.


## Run index for visual clients

The live Control Plane API can enumerate known runs:

~~~text
GET /runs
~~~

The response is ordered by most recent event activity and contains projected summaries rather than raw directory metadata.

Each summary includes:

- run ID;
- run status;
- last event sequence;
- deterministic task progress;
- agent count;
- message/artifact counts;
- pending approvals;
- last event timestamp.

The index is still derived from the authoritative event streams: every listed run is replayed through the same state projector used by `/runs/<run-id>/state`.

Unsafe/non-run filesystem entries are ignored, while a symlink pretending to be a valid run directory is rejected.


## Collaboration detail endpoints

The local Control Plane server can expose the real collaboration records for a run:

~~~text
GET /runs/<run-id>/messages
GET /runs/<run-id>/artifacts
~~~

### Messages

The messages endpoint reads the persisted MessageBus history for the requested run, including expired messages for audit/reconstruction.

It exposes the typed provider-independent envelope:

- sender;
- target;
- kind;
- priority;
- reply relationship;
- artifact references;
- sequence;
- timestamps.

### Artifacts

The artifacts endpoint reads immutable ArtifactStore entries filtered by `run_id` and ordered by creation time.

No separate UI database is introduced.

### Server configuration

`ai run serve` now opens the same collaboration stores used by coordinated execution.

Optional overrides:

~~~text
--artifacts-dir <path>
--messages-dir <path>
~~~

Otherwise the normal environment/user-config resolution applies.

This lets a future visual client render conversations, decisions, handoffs, verification reports and API contracts from durable platform data rather than provider chat history.
