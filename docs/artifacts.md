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


## Typed high-value payloads

The artifact envelope remains generic, but high-value coordination artifacts now validate dedicated payload contracts.

### ArchitectureDecision

Required:

- `decision`
- `rationale`

Optional bounded lists:

- `constraints`
- `interfaces`
- `unknowns`

### APIContract

Required:

- version;
- HTTP/HTTPS/REST protocol;
- one or more operations.

Every operation has a unique ID, supported HTTP method, absolute API path, and optional bounded request/response schema summaries.

### VerificationReport

Required:

- terminal report status: `passed`, `failed`, `partial`, or `infrastructure`;
- one or more uniquely identified checks.

Checks have explicit status, summary, and optional bounded evidence.

### Strict validation

For these artifact types:

- unknown JSON fields are rejected;
- trailing/multiple JSON values are rejected;
- duplicate operation/check IDs are rejected;
- list and text sizes are bounded;
- validation occurs both when creating and when loading persisted artifacts.

This lets collaborating agents treat these artifacts as stable contracts rather than arbitrary JSON blobs.

Other artifact types retain the generic payload contract until their dedicated schema is introduced.
