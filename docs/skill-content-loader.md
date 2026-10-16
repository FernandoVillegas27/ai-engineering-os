# Skill Content Loader

## Purpose

The Skill Content Loader turns a resolved skill registry entry into the actual skill instructions that an agent can consume.

The resolver answers:

> Which skill is relevant?

The content loader answers:

> Where is the exact skill content, and can it be loaded safely and reproducibly?

## Supported sources

The initial implementation supports two source families.

### External GitHub skills

External GitHub skills are loaded from the repository, path and pinned Git revision stored in the Skill Registry.

Example:

~~~yaml
source:
  type: external
  provider: gentleman-skills
  repository: Gentleman-Programming/Gentleman-Skills
  path: community/spring-boot-3/SKILL.md
  version: git:c8036a37893679dc5e942484975405d39689c63b
~~~

The loader requests that exact revision rather than the moving `main` branch.

### Local company or generated skills

Company and generated skills can be loaded from a configured local base directory.

Their paths are resolved relative to that directory and path traversal outside the base directory is rejected.

## Candidate policy

Candidate skills are not loaded by default.

A caller must explicitly enable candidate loading.

This preserves the trust boundary established by the Skill Resolver:

~~~text
matching skill
    ↓
candidate
    ↓
explicit approval
    ↓
content loading
~~~

An enabled skill does not require that extra candidate override.

## Content validation

After loading a `SKILL.md`, the loader parses its YAML frontmatter.

The frontmatter `name` must match the registry skill ID.

Example:

~~~text
registry id:
spring-boot-3

SKILL.md frontmatter:
name: spring-boot-3

→ accepted
~~~

A mismatch is rejected because it can indicate incorrect registry metadata or unexpected upstream content.

## Reproducibility

External GitHub content must use:

~~~text
git:<revision>
~~~

The current loader accepts hexadecimal commit IDs and rejects unpinned external versions.

This prevents a future change on an upstream default branch from silently changing the knowledge used by an agent.

## Security boundaries

The loader currently enforces:

- explicit candidate approval;
- no disabled or deprecated skill loading;
- pinned external Git revisions;
- repository names in `owner/name` format;
- safe external paths;
- local path traversal protection;
- a bounded response size for remote content;
- frontmatter identity validation.

## What the loader returns

The result contains:

- skill ID and frontmatter name;
- description;
- author;
- skill version;
- source metadata;
- pinned revision;
- original path;
- parsed body;
- raw `SKILL.md` content.

This is the object that a future runtime adapter can inject into an agent context.

## Current boundary

The loader does not decide which skill should be used.

That remains the responsibility of the Skill Resolver.

It also does not yet:

- cache external skill content;
- verify cryptographic checksums;
- execute an agent;
- project skills into Cursor, Claude Code or Codex;
- use Gentle-AI.

Those concerns remain separate.
