# Durable Run Task Graph

## Purpose

Retry and resume must not rebuild dependencies from a new classifier pass.

The durable run record now stores the same deterministic dependency graph used by the scheduler.

## Binding

After the task graph is built, `runstate.BindTaskGraph` validates and persists:

- work-unit identity;
- agent identity;
- dependency IDs;
- initial scheduler status.

The binding does not change:

- permissions;
- runtime/provider selection;
- capabilities;
- task text.

## Read-only runs

The read-only coordinated path now builds the task graph before the first run-state save and binds it into the durable run.

## Coordinated mutation runs

Mutation runs persist only the tasks that actually participate before promotion:

- read-only pre-stage ancestors;
- source-writer implementation units;
- tests-only QA units.

The participant subgraph must preserve every dependency of every included task. If an included task depends on an excluded task, run creation fails instead of silently dropping that dependency.

Reviewer remains a run-wide gate rather than a fake mutating work unit.

## Why this matters

A later process can reconstruct the execution graph from the persisted run record without:

- chat context;
- re-running classification;
- re-planning agent ordering;
- guessing which successful dependency a failed task required.

This is the prerequisite for targeted retry and durable resume.
