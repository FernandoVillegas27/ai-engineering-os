# External Skill Catalog

## Purpose

This catalog is the first controlled registration of external skills in AI Engineering OS.

The platform does not vendor or rewrite the upstream skill content in this step. It records metadata that lets the core reason about provenance, compatibility, trust and activation.

## Source

The initial entries come from:

`Gentleman-Programming/Gentleman-Skills`

All entries are pinned to the same upstream commit:

`c8036a37893679dc5e942484975405d39689c63b`

This makes the catalog reproducible. A future upstream change does not silently change our registered knowledge.

## Initial entries

The first catalog contains:

- `angular-core`
- `angular-architecture`
- `typescript`
- `playwright`
- `pytest`
- `django-drf`
- `java-21`
- `spring-boot-3`

This is intentionally a small subset.

## Why every skill is still a candidate

External publication quality is not the same thing as company approval.

The upstream files were inspected for provenance and shape, but they have not gone through our own engineering approval process.

Some skills also contain opinionated rules. For example, framework guidance can encode conventions that may be valid for one team but inappropriate for another.

For that reason, the initial entries use:

~~~yaml
trust:
  reviewed: false
status: candidate
~~~

This lets the future resolver discover them without presenting them as company-approved knowledge.

## Curated vs community

The upstream repository separates some skills into curated and community areas.

We preserve that distinction in our trust metadata:

~~~text
upstream curated
→ trust.level: curated

upstream community
→ trust.level: community
~~~

We still retain our own independent `reviewed` flag and lifecycle status.

## No vendor lock-in

The registry stores logical metadata around the external content.

It does not make Gentleman-Skills part of the AI Engineering OS core.

A future catalog may mix:

~~~text
Gentleman-Skills
company skills
another open-source catalog
generated skills
specialized vendor skills
~~~

All sources must enter through the same Skill contract.

## Promotion workflow

The intended lifecycle is:

~~~text
candidate
   ↓
engineering review
   ↓
approved metadata or company override
   ↓
enabled
~~~

If the external guidance conflicts with company standards, we can keep the external skill disabled and create a company skill with higher trust.

## Update workflow

A future update command should:

1. detect a newer upstream commit;
2. compare registered skill changes;
3. show the diff;
4. require review when appropriate;
5. update the pinned commit only after approval.

Automatic movement to the latest upstream branch is intentionally avoided.
