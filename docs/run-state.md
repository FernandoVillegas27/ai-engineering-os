# Durable Run State

## Purpose

A ChatGPT, Cursor, Codex, Claude or IDE session must not be the source of truth for an engineering run.

P4a introduces a provider-independent, versioned run record that can be saved, loaded and inspected by another process later.

This is the persistence foundation for future:

- `ai run status`;
- pause/resume;
- task graphs;
- agent messages;
- artifacts;
- approvals;
- control-plane views.

It does not introduce the multi-agent scheduler yet.

## Run identity

A run has stable generated identifiers:

- `run_id`;
- `task_id`.

Work units keep their stable execution IDs.

IDs are data identifiers, not provider conversation IDs.

## Persisted context

The initial record can preserve:

- original task;
- project identity;
- project root reference;
- requested capabilities;
- runtime/provider/model policy selection;
- selected work units and agents;
- enabled skills and logical tools;
- effective permission envelopes;
- work-unit dependencies;
- artifact/message references;
- quality summary;
- approvals;
- lifecycle timestamps.

Provider conversation transcripts are deliberately not required.

## Run states

Initial lifecycle:

~~~text
planned
  -> running
  -> paused -> running
  -> completed
  -> failed
  -> cancelled
~~~

Terminal runs cannot silently return to running.

## Work-unit states

The persisted contract already reserves the control-plane states:

- `queued`;
- `ready`;
- `running`;
- `waiting`;
- `blocked`;
- `completed`;
- `failed`;
- `cancelled`.

P4a stores them. A later scheduler will own their transitions.

## Persistence

The initial store uses one versioned JSON document per run in an explicitly configured storage directory.

Properties:

- schema version is required;
- unknown JSON fields are rejected when loading;
- invalid state is rejected before saving;
- unsafe run IDs cannot become paths;
- symlink run-state entries are rejected;
- writes use a temporary file and replacement strategy;
- storage is independent from any model or IDE.

The storage root is explicit. The core package does not silently create state inside an arbitrary source repository.

## Current boundary

P4a does not yet:

- wire persistence into every CLI execution path;
- implement event sourcing;
- implement a task scheduler;
- resume provider conversations;
- persist message/artifact bodies;
- write project memory;
- add a database.

Those are later phases.

The key invariant introduced here is:

> A run has a durable, versioned state that can be reconstructed without the chat that created it.
