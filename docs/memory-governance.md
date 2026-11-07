# Memory governance

## Purpose

Automatic project learning is useful only if it remains inspectable, reversible, bounded, and subordinate to the current run's authority.

AI Engineering OS therefore treats the local governed memory ledger as the authority for automatic recall.

External memory providers such as Engram may receive a redacted mirror, but provider-native memory is not automatically trusted or recalled outside the governance ledger.

## Authority model

~~~text
current run task / permissions / system policy
                ↓ authoritative
durable governed project memory
                ↓ untrusted historical evidence
optional external provider mirror
~~~

A memory record can never:

- change the current task;
- grant tool access;
- broaden filesystem/network permissions;
- select a model for a role;
- bypass Quality or review;
- authorize promotion;
- override cancellation/recovery/integrity rules.

## Record schema

Each governed record contains:

- immutable memory ID;
- project namespace;
- title and redacted content;
- memory type;
- stable topic key;
- provider/mirror identity;
- provenance:
  - source run;
  - source artifact;
  - source task;
  - producer agent;
  - source kind;
- status:
  - active;
  - superseded;
  - deleted;
- created/updated timestamps;
- optional expiration timestamp;
- supersession link;
- delete reason;
- SHA-256 of the governed content.

The ledger uses one hashed directory per project and rejects symlinked project directories and symlinked record files.

## Retention policy

Automatic curated memory uses bounded defaults:

| Type | Default retention |
| --- | ---: |
| architecture / decision | 365 days |
| discovery / bugfix / pattern / config | 180 days |
| run learning | 90 days |

Expired records remain inspectable with `--all`, but are excluded from automatic recall.

## Sensitive-content redaction

Before a record enters the governed ledger or an external mirror, common secret forms are redacted, including:

- Authorization Bearer values;
- API keys;
- tokens;
- secrets;
- passwords;
- GitHub-style personal tokens;
- OpenAI-style secret keys;
- PEM private keys.

The redacted content is what receives the stored SHA-256.

A secret discovered later should still be rotated at its source. Redaction is a containment layer, not credential revocation.

## Review and lifecycle CLI

Status:

~~~text
ai memory status
~~~

List active memories:

~~~text
ai memory list --project <project-id>
~~~

Audit active + expired + superseded + deleted records:

~~~text
ai memory list --project <project-id> --all
~~~

Inspect one record and its provenance:

~~~text
ai memory show <memory-id> --project <project-id>
~~~

Delete from future automatic recall:

~~~text
ai memory delete <memory-id> --project <project-id> --reason "obsolete"
~~~

Supersede while preserving history:

~~~text
ai memory supersede <memory-id> \
  --project <project-id> \
  --content "replacement knowledge" \
  --type architecture \
  --retention 8760h
~~~

All lifecycle operations also support `--json` where applicable.

Delete is a tombstone, not silent evidence destruction. Supersede creates a new record and links the old record to it.

## Topic convergence

Automatic artifacts and run learnings already use stable topic keys.

When a governed provider receives the same topic again:

- identical redacted knowledge reuses the current record;
- changed knowledge creates a replacement and supersedes the old record.

This prevents unbounded duplicate memories while retaining an audit trail.

## Project isolation

All lookup, recall, delete and supersede operations require the project namespace.

A record from one project cannot be loaded or recalled through another project's namespace.

Project directory names are derived from a cryptographic hash of the namespace rather than raw user-controlled path fragments.

## External provider behavior

Engram remains an optional mirror/provider integration.

Automatic recall uses only governed local records. This is deliberate:

- deleting a governed record really removes it from automatic agent context;
- provider-native memories that were never governed cannot silently re-enter;
- an unavailable external provider does not disable local governed memory;
- mirror failures are warnings after the governed save succeeds.

This boundary makes memory reversible independently of external provider capabilities.
