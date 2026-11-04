# Post-Integration Tests-Only QA

## Purpose

QA has `tests: write` but not `source_code: write`.

The coordinated mutation pipeline now gives tests-only agents a separate downstream mutation phase after source-writer outputs have already been integrated.

This preserves both the permission boundary and the specialist DAG.

## Execution order

~~~text
Researcher / Architect
  -> source-writer proposals
  -> isolated source stages
  -> source integration workspace
  -> source-integration handoff
  -> tests-only QA proposal
  -> tests-only isolated stage
  -> tests overlay workspace
  -> run-wide Quality Gate
  -> promotion candidate
~~~

QA is never included in the parallel source-writer stage.

## Source integration handoff

After source integration, the orchestrator persists one immutable handoff artifact describing the integrated source change set.

Each tests-only agent receives a typed handoff message referencing that artifact.

The agent loads its bounded inbox before Proposal Freeze, so its test plan observes the combined implementation rather than individual provider conversations.

## Proposal enforcement

Tests-only units are selected from registry-derived metadata:

- `tests: write`;
- `source_code != write`.

Their proposals still use the normal read-only ProposalRuntime.

After Proposal Freeze, every declared path must pass the conservative test file-kind policy.

A mixed test/source proposal is rejected before Stage.

## Stage

Tests-only StageRuntime executions use the source integration workspace as their project root.

Each provider receives a sanitized isolated copy.

The existing frozen-stage adapter still:

- re-snapshots the filesystem;
- verifies provider-reported changes against actual changes;
- enforces the frozen declared scope;
- freezes a ResultImage.

## Test overlay

QA changes are not copied back into the source integration workspace.

Instead, the platform creates a fresh final integration workspace from the source-integrated result.

For every tests-only workspace it requires:

- the same source-integration pre-image;
- frozen ResultImage matching inspection;
- tests-only path classification;
- declared-scope match;
- no path overlap with existing source changes;
- no duplicate test ownership across tests-only agents.

The final workspace preserves the original project's PreImage while its final change set becomes:

~~~text
source changes + tests-only changes
~~~

This allows the existing baseline provider to continue comparing against the original project state.

## Quality

`Integration.RunQualityGate` now accepts an optional explicit final expected-change set.

Existing callers retain the previous plan-derived behavior.

For source+QA runs, the gate receives the final union of source and tests changes, then runs the same baseline-aware verification, correction and independent review.

## Safety

Tests-only agents cannot:

- declare source files;
- modify the source integration workspace directly;
- overlap a source-writer change;
- broaden `tests: write` into `source_code: write`;
- bypass final run-wide Quality;
- apply changes to the original project.

Promotion remains a separate explicit approval boundary.
