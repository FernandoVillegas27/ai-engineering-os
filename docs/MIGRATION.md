# Migration guide

## Release line

The first release-candidate line is `0.1.0-rc.1`.

Until a stable `1.0` contract exists, persisted schema changes must remain versioned and backward-aware.

## Before upgrading

1. Finish or explicitly pause active runs.
2. Keep the durable run/event/artifact/message directories intact.
3. Record the current binary identity with `ai version`.
4. Run `ai doctor .`.
5. Keep a normal filesystem backup of user configuration data before changing release lines.

Do not manually edit snapshot, compacted-history, lease, checkpoint or promotion-candidate records.

## Event history

The current release understands:

- append-only event histories;
- integrity-verified snapshots;
- external archives;
- versioned compacted history.

Historical evidence remains authoritative. A snapshot is not permission to discard unverified event evidence.

## Typed artifact/message payloads

Current lifecycle payloads are versioned.

Older unversioned handoff records remain readable under the explicit read-legacy/write-current rule. New writes should use the current typed contracts.

## Memory migration

Earlier builds could use Engram directly as the automatic project-memory provider.

The governed-memory release changes the authority model:

~~~text
old:
  automatic recall -> external provider

new:
  automatic recall -> local governed ledger
  external provider -> optional redacted mirror
~~~

Provider-native records are not imported automatically because doing so would bypass provenance/redaction/review requirements.

If old provider knowledge is still important, review it and deliberately recreate the durable facts through governed memory rather than bulk-importing transcripts.

## Runtime/model configuration

Do not migrate by pinning a model to each agent role.

Keep provider-independent policies and use an explicit model override only for a concrete runtime-specific need.

## Project metadata

Re-running:

~~~sh
ai inspect . --write
~~~

regenerates project detection/skill recommendations. Review resulting changes before committing them.

## Rollback

If a new release candidate is unsuitable:

- stop active processes;
- restore the previous binary;
- do not downgrade persisted data by manually editing schema versions;
- preserve all durable state for diagnosis.

If a newer release writes a schema unsupported by the older binary, use the newer binary for inspection/export rather than coercing the data backward.

## Stable-release expectation

A stable release should define:

- supported persisted schema migration guarantees;
- minimum Go/runtime prerequisites;
- release artifact checksums;
- compatibility matrix;
- upgrade/downgrade policy;
- signed or otherwise verifiable release provenance where available.
