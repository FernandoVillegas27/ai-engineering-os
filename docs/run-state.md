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


## CLI inspection

The durable state can be inspected independently from the provider that created it.

Initial commands:

~~~text
ai run init --task "..." --project my-project
ai run list
ai run status <run-id>
ai run transition <run-id> --status running
ai run transition <run-id> --status paused
~~~

Use `--json` for machine-readable output.

Storage resolution order:

1. explicit `--state-dir`;
2. `AI_ENGINEERING_OS_STATE_DIR`;
3. the operating system user configuration directory under `ai-engineering-os/runs`.

This keeps run state outside source repositories by default while still allowing deterministic explicit storage for tests, automation and future control-plane services.

These commands manipulate only run metadata. They do not resume an AI provider session or execute project code.


## Coordinated execution

The first integrated run path is:

~~~text
ai run execute [project-path] --task "..." --dry-run
ai run execute [project-path] --task "..." --runtime codex
~~~

This command now connects the previously separate foundations:

1. task classification;
2. deterministic orchestration plan;
3. execution work units;
4. durable run creation;
5. deterministic scheduler graph;
6. bounded multi-agent coordinator;
7. Control Plane event persistence;
8. durable terminal run/work-unit state.

The initial integrated path is deliberately **read-only**.

Selected agents may execute concurrently when the scheduler permits it, but no agent in this path receives write access to source code, tests, Git, secrets or production.

Use:

~~~text
ai run control <run-id>
ai run events <run-id>
ai run status <run-id>
~~~

after execution to inspect the same run from the durable state and Control Plane.

### Specialist collaboration DAG

The first deterministic role-aware graph is now active for selected agents.

When the corresponding roles are present, the default collaboration order is:

~~~text
Researcher
   |
Architect
   |
Backend / Frontend / Database
   |
QA
   |
Reviewer
~~~

Missing roles are skipped rather than invented.

Examples:

- without an Architect, implementation roles may depend directly on Researcher when Researcher is selected;
- QA depends on selected implementation roles;
- Reviewer waits for every other selected work unit;
- unknown future roles remain independent until an explicit collaboration rule is defined.

These are deterministic platform rules, not model-generated scheduling decisions.

Future Orchestrator v2 decomposition can replace or refine this default graph with an explicit validated task DAG for a specific objective.

Write-capable multi-agent execution remains intentionally out of scope until isolated per-task staging and merge/conflict coordination are implemented.
