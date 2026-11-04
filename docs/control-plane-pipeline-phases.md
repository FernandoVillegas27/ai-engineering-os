# Control Plane Mutation Pipeline Phases

## Purpose

A visual/operational client should know which mutation phase is active without parsing human summaries.

The Control Plane now has typed lifecycle events for:

- proposal;
- isolated stage;
- integration.

## Events

Each phase supports:

~~~text
<phase>.started
<phase>.completed
<phase>.failed
~~~

Current event names:

- `proposal.started`
- `proposal.completed`
- `proposal.failed`
- `stage.started`
- `stage.completed`
- `stage.failed`
- `integration.started`
- `integration.completed`
- `integration.failed`

## Projected state

The Control Plane state exposes:

~~~json
{
  "pipeline": {
    "proposal": "completed",
    "stage": "running",
    "integration": ""
  }
}
~~~

Values are empty until observed and then become:

- `running`;
- `completed`;
- `failed`.

## Source and tests-only phases

The mutation runner emits these lifecycle events both for:

- source-writer mutation;
- post-integration tests-only QA.

The event's structured `data.phase` distinguishes:

~~~text
source
tests-only-qa
~~~

The projected aggregate state reflects the latest lifecycle state for each phase, while the immutable event history preserves both source and QA passes.

## Failure visibility

Provider/proposal failures, StageRuntime failures, integration-plan failures, materialization failures and tests-only overlay failures emit the corresponding typed failed event before the run is terminated.

This lets clients show a real failure location instead of inferring it from the final run status.

## Relationship to Quality

Quality remains a separate lifecycle family:

- verification;
- baseline comparison;
- correction;
- review.

Pipeline events describe how the candidate workspace is constructed.

Quality events describe whether that constructed candidate is acceptable.
