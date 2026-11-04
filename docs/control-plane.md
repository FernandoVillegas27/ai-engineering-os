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
