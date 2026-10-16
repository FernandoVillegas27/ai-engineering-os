# Skill Contract

## What a skill is

A skill is reusable knowledge that can be attached to an agent for a specific task or project.

A skill is not an agent and it is not a model.

Examples of skills we may register later include `spring-boot-3`, `java-21`, `angular-core`, `playwright`, `pytest`, and internal company standards.

The key distinction is:

~~~text
Agent = stable engineering role
Skill = specialized knowledge
~~~

For example:

~~~text
backend-engineer
  + java-21
  + spring-boot-3
  = backend engineer specialized for this task
~~~

## Why skills are separate from agents

Technology-specific knowledge changes much faster than engineering roles. The system keeps a small set of stable roles and dynamically attaches the knowledge required by the current project.

## Skill source

Every skill records where it came from. Supported source types are:

- `company`: maintained by our organization;
- `external`: imported from a third-party repository or catalog;
- `generated`: created with AI and not trusted by default.

External systems such as Gentleman-Skills can be used as sources without becoming part of the core platform.

## Trust model

Skills have an explicit trust level with the intended precedence:

~~~text
company
  ↓
curated
  ↓
community
  ↓
generated
~~~

Lower-trust skills are not automatically rejected. The resolver and quality gates can simply treat them more cautiously.

## Review policy

Generated or community skills should not become trusted production guidance merely because they exist. Review state is explicit and can be used by policy later.

## Capabilities

Skills reinforce one or more capabilities. A Spring Boot skill, for example, may support `backend-development` and `api-development`.

## Domains

Domains describe broad knowledge areas such as `backend`, `frontend`, `database`, `testing`, `architecture`, and `security`.

Agents expose `skill_domains`, and skills expose `domains`. This gives the resolver a deterministic compatibility signal.

## Technologies and versions

A skill may declare the technologies and versions it covers. Version compatibility matters because framework guidance can become incorrect across major versions.

## Compatible agents

A skill explicitly declares which agent roles may use it. This prevents irrelevant skills from being attached to unrelated roles.

## Tools

Skills may require or recommend tools. A required tool blocks activation when unavailable; a recommended tool improves the result but is not mandatory.

## Activation hints

Skills can expose triggers and project signals that help the Project Detector and Skill Resolver. These are hints, not proof; deterministic project evidence should be preferred when possible.

## Registry metadata vs skill content

The registry does not replace the original skill format. An external skill may keep its native `SKILL.md` exactly as published, while our registry adds platform metadata around it.

~~~text
external SKILL.md
       +
AI Engineering OS registry metadata
       =
usable skill in our platform
~~~

This avoids forking upstream formats unnecessarily.

## Versioning external skills

External skills should eventually be pinned to a concrete version, tag or commit. We should avoid silently consuming a moving upstream branch in production.

## Validation

The machine-readable contract is `schemas/skill.schema.json` and the registry is `registry/skills.yaml`.

The registry is intentionally empty at this step: we define the contract first and import external skills later.

## Design rules

1. Skills provide knowledge; agents provide roles.
2. Skills remain independent from AI model vendors and IDEs.
3. External skill formats should be preserved when practical.
4. Registry metadata enriches upstream skills instead of rewriting them.
5. Trust and review state must be explicit.
6. Technology version compatibility must be representable.
7. Generated skills are never implicitly trusted.
8. External dependencies should be version-pinned before production use.
9. A skill should support known capabilities and compatible agents.
10. Skill activation should be deterministic whenever project evidence is available.