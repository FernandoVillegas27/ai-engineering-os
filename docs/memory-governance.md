# Project Memory Governance

## Purpose

Automatic team learning must remain inspectable, reversible, bounded, and unable to override execution policy.

AI Engineering OS now places a governance layer in front of project-memory writes. Engram remains the search/storage provider, while the OS records its own append-only evidence for every memory it creates.

## Automatic save lifecycle

A governed save is two-phase:

1. redact sensitive content;
2. append a local `pending` governance record;
3. call the memory provider;
4. require a stable provider memory ID;
5. append a `saved` or `superseded` confirmation.

If provider delivery fails, times out, is uncertain, or does not return a stable ID, the local record remains `pending`.

A pending record is not treated as active memory and is not silently retried.

## Provenance

Automatic curated memories carry available provenance:

- run ID;
- task ID;
- agent ID;
- source kind.

Run-level learning identifies its originating run and uses source `run-learning`.

The governance ledger also records:

- provider;
- provider memory ID;
- stable topic key;
- redacted title/content;
- content SHA-256;
- type;
- lifecycle timestamps;
- review, supersession, and deletion evidence.

## Sensitive-content redaction

Before provider delivery, the current deterministic redactor removes common credential material including:

- private-key blocks;
- Bearer tokens;
- API/access/auth token assignments;
- password/secret assignments;
- GitHub token formats;
- AWS access-key IDs.

Redaction is defense in depth. Agents must still avoid putting secrets into durable learning.

Raw prompts and conversation transcripts are not part of the automatic curated-memory lifecycle.

## Project boundary

Governance storage is project-scoped through a deterministic hash of the project namespace, never a user-controlled filesystem path.

Every ledger entry revalidates the project identity when loaded.

Engram observation deletion is ID-based, so the adapter first performs a project-scoped observation lookup before invoking deletion. A memory cannot be deleted through another project's governance ID.

## Human lifecycle commands

Inspect:

~~~text
ai memory list --project <id>
~~~

Mark a governed record as reviewed:

~~~text
ai memory review <governance-id> --project <id>
~~~

Replace an active memory while preserving its stable topic lineage:

~~~text
ai memory supersede <governance-id> --project <id> \
  --title "New decision" \
  --content "Updated durable knowledge"
~~~

Soft-delete one active memory:

~~~text
ai memory delete <governance-id> --project <id>
~~~

Hard deletion requires an additional explicit confirmation:

~~~text
ai memory delete <governance-id> --project <id> --hard --yes
~~~

Retention is never applied as a side effect of reads or normal runs. Pruning is an explicit bulk operation and always requires `--yes`:

~~~text
ai memory prune --project <id> --older-than 90d --yes
~~~

Optional hard pruning:

~~~text
ai memory prune --project <id> --older-than 90d --hard --yes
~~~

## Supersession

Automatic or manual writes with the same non-empty `topic_key` create a new governed record and supersede the previously active record.

The ledger retains both records and the forward/back references.

Only active records can be deleted by the governed deletion path.

## Storage

Governance ledger root resolution:

1. `AI_ENGINEERING_OS_MEMORY_GOVERNANCE_DIR`;
2. user config directory under `ai-engineering-os/memory-governance`.

Ledger entries are immutable sequential JSON files written with exclusive creation and fsync.

Loading fails closed on:

- sequence gaps;
- symlink/non-regular entries;
- project mismatch;
- invalid lifecycle references;
- modified governed content digest;
- unsupported schema/action.

## Trust boundary

Recalled project memory remains explicitly labeled **untrusted historical evidence**.

It cannot change:

- the requested task;
- permissions;
- model/runtime policy;
- safety rules;
- tool availability;
- Quality gates;
- promotion approval.

Memory governance improves durable learning; it does not make memory authoritative.
