# Agent Artifact Protocol

## Purpose

Agents should exchange durable engineering work products instead of replaying entire model conversations.

An artifact is a provider-independent, versioned record produced by an agent during a run.

Examples:

- architecture decision;
- API contract;
- implementation plan;
- verification report;
- review report;
- handoff.

## Initial artifact types

- `task-brief`
- `architecture-decision`
- `research-note`
- `implementation-plan`
- `change-proposal`
- `code-patch-reference`
- `test-plan`
- `verification-report`
- `review-report`
- `risk-assessment`
- `question`
- `decision`
- `handoff`
- `api-contract`

## Envelope

Each artifact contains:

- schema version;
- stable artifact ID;
- run ID;
- task ID;
- producer agent;
- artifact type;
- lifecycle status;
- concise human-readable summary;
- JSON structured payload;
- related artifact IDs;
- evidence references;
- creation/update timestamps.

The payload is deliberately typed only by artifact type at this stage. Later PRs may add dedicated JSON Schemas for high-value artifact types such as API contracts and verification reports.

## Immutability

Persisted artifact IDs are immutable.

A stored artifact cannot be overwritten in place. Revisions should create a new artifact and reference/supersede the previous artifact.

This makes agent handoffs and audit history reproducible.

## Storage safety

The initial local store:

- uses an explicit storage directory;
- rejects unsafe IDs;
- rejects symlink entries;
- rejects malformed/unknown JSON fields on load;
- limits summary and payload sizes;
- stores one artifact per file.

## Relationship to messages

Artifacts contain durable engineering content.

Messages coordinate agents and should reference artifact IDs instead of embedding large repeated payloads.

Example:

~~~text
architect
  -> artifact: ArchitectureDecision #A

backend-engineer
  -> artifact: APIContract #B
  -> message to frontend-engineer referencing #B

qa-engineer
  -> artifact: VerificationReport #C
  -> message to backend-engineer referencing #C
~~~

The typed inter-agent message bus is the next phase.
