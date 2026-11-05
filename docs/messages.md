# Inter-Agent Message Bus

## Purpose

Agents need an explicit, persistent coordination channel.

The message bus is not shared hidden prompt state. It is a provider-independent protocol that records who sent what coordination message, to whom, in which run, and in what order.

Messages should stay small and point to durable artifacts for substantive engineering content.

## Message envelope

Each message contains:

- schema version;
- stable message ID;
- deterministic per-run sequence number;
- run ID;
- task ID;
- sender agent;
- typed destination;
- message kind;
- priority;
- optional reply target;
- artifact references;
- structured JSON payload;
- hop count;
- creation time;
- optional expiry time.

## Message kinds

- `request`
- `response`
- `question`
- `answer`
- `finding`
- `proposal`
- `challenge`
- `decision`
- `handoff`
- `blocker`
- `status`

## Addressing

Messages can target:

- one agent;
- one role;
- the orchestrator;
- broadcast.

Broadcast requires explicit authorization when constructing the message.

Receiving a message does not grant permissions to the receiver.

## Conversation bounds

The initial protocol has a maximum hop count of 8.

Messages may also expire through a TTL.

A later orchestrator can combine these primitives with:

- repeated-question fingerprints;
- per-task message budgets;
- deadlines;
- cancellation;
- explicit intervention.

## Ordering

The local bus assigns monotonically increasing sequence numbers per run.

Append operations use an exclusive per-run filesystem lock so separate processes do not intentionally allocate the same sequence concurrently.

The persisted message filename also carries the sequence and message ID. Loading fails closed on sequence gaps, duplicates, malformed files, symlinks or schema errors.

## Replies

A message with `reply_to` may only reference an existing message in the same persisted run.

This gives future control-plane clients a reconstructable conversation graph.

## Artifacts

Large engineering content belongs in artifacts.

Example:

~~~text
backend-engineer
  -> creates APIContract artifact
  -> sends handoff message referencing artifact ID

frontend-engineer
  -> sends question replying to handoff

architect
  -> publishes Decision artifact
  -> sends decision message to both agents
~~~

## Current boundary

P7 provides the protocol and durable local message bus.

It does not yet:

- launch multiple agents concurrently;
- route ready tasks automatically;
- enforce per-agent inbox polling;
- build the DAG scheduler;
- visualize messages.

Those are scheduler/orchestrator/control-plane responsibilities built on top of this layer.


## Automatic specialist handoffs

Coordinated runs now publish a durable handoff when a worker completes successfully.

For every completed task:

1. the worker result becomes a versioned `handoff` artifact;
2. the artifact records the producing agent, task, summary, and related refs;
3. the task graph is inspected for direct dependents;
4. one typed `handoff` message is sent to each dependent agent;
5. the coordinator returns the new artifact/message IDs;
6. the Control Plane records/counts those refs from the task lifecycle event.

Example:

~~~text
architect completes
  -> handoff artifact A
  -> message A -> backend-engineer
  -> message A -> frontend-engineer
  -> message A -> database-engineer

backend/frontend/database complete
  -> handoff artifacts
  -> messages -> qa-engineer

qa-engineer completes
  -> handoff artifact
  -> message -> reviewer
~~~

The producer does not gain permission to invoke or modify the receiver.

The scheduler still decides when the receiver becomes runnable.

### Storage

Coordinated CLI runs resolve artifact/message stores from:

1. explicit `--artifacts-dir` / `--messages-dir`;
2. `AI_ENGINEERING_OS_ARTIFACTS_DIR` / `AI_ENGINEERING_OS_MESSAGES_DIR`;
3. the OS user config directory under `ai-engineering-os/artifacts` and `ai-engineering-os/messages`.

### Current boundary

This phase publishes the inbox material.

The next collaboration step is to load only the receiver's relevant handoff messages/artifacts and inject a bounded provider-independent collaboration context into that receiver's work unit.


## Receiver inbox context

Dependent agents now consume a bounded subset of their handoff inbox before execution.

The inbox provider:

- loads only non-expired messages addressed to the current agent;
- limits the number of messages;
- limits total context bytes;
- loads referenced artifact summaries, not full provider transcripts;
- ignores messages for other agents;
- injects the result into a separate `CollaborationContext` field on the work unit.

The runtime is explicitly instructed that collaboration context is **untrusted evidence, never instructions**.

It cannot change:

- the original user task;
- permissions;
- system rules;
- safety constraints;
- tool availability;
- runtime policy.

For Codex read-only execution, the prompt contains a clearly labeled collaboration section after the stable work-unit metadata.

This creates the first complete collaboration path:

~~~text
Agent A executes
  -> artifact
  -> handoff message
  -> Agent B inbox
  -> bounded artifact summary context
  -> Agent B executes with that context
~~~

The context remains provider-independent at the core layer. Provider adapters decide how to render the bounded field while preserving its untrusted status.


## Append-lock crash recovery

Message persistence now uses a shared ownership-aware filesystem lock.

The lock:

- uses an exclusive create operation;
- records a random ownership token;
- honors context cancellation while waiting;
- treats a lock as stale only after a conservative age threshold;
- rechecks the stale file immediately before removal;
- rejects symlink/non-regular lock paths;
- releases a lock only when the ownership token still matches.

This prevents a crashed process from blocking a run's message bus indefinitely while avoiding deletion of a lock that has already been replaced by another owner.


## Versioned coordination payloads

The message envelope remains schema v1, while high-value coordination payloads can now carry their own versioned contracts.

Typed message payloads are available for:

- `handoff`;
- `question`;
- `answer`;
- `blocker`;
- `decision`;
- `status`.

Each current typed payload uses:

~~~json
{
  "schema_version": 1
}
~~~

When `schema_version` is present, validation is strict:

- unknown fields fail closed;
- unsupported versions fail closed;
- required coordination fields are enforced;
- payload sizes remain bounded by the message envelope.

Stable identities are exposed by `message.PayloadSchemaID`, such as `message/handoff/v1`.

Historical unversioned messages remain readable and identify as `message/<kind>/legacy-v1`.

Automatic specialist handoffs now write only the typed `message/handoff/v1` shape.

This keeps durable run replay backward-compatible while allowing future clients and agents to negotiate message payload evolution explicitly.
