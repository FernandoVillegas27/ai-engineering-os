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
