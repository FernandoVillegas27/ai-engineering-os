# Skill Resolver

## Purpose

The Skill Resolver connects project detection with the Skill Registry.

It answers:

> Given the technologies detected in this project, which registered skills are relevant?

The resolver is deterministic. It does not call an AI model.

## Inputs

The resolver receives:

1. technology detection produced by the Project Detector;
2. the Skill Registry;
3. optional filters such as agent role or required capabilities.

## Outputs

The result is split into three groups:

- `Enabled`: matching skills that are approved and enabled;
- `Candidates`: matching skills that are known but not yet approved for automatic use;
- `UnmatchedTechnologies`: detected technologies for which no compatible registered skill matched.

This distinction is important.

A matching `candidate` skill is discoverable, but it is not silently promoted into trusted runtime knowledge.

## Example

Given:

~~~text
Detected:
  java@21
  spring-boot@3
~~~

and a registry containing:

~~~text
java-21         candidate
spring-boot-3   enabled
~~~

the resolver returns:

~~~text
Enabled:
  spring-boot-3

Candidates:
  java-21
~~~

## Version compatibility

If a skill declares supported versions, the detected version must match.

Example:

~~~text
java-21 skill
supports: 21

project:
java: 17

result:
no match
~~~

A skill without explicit versions can match any detected version of that technology.

## Agent compatibility

The resolver can optionally filter for an agent.

Example:

~~~text
angular-core
compatible agents:
  frontend-engineer
  architect
  reviewer
~~~

If resolution is requested for `backend-engineer`, that skill is not returned.

## Capability compatibility

The resolver can also optionally require overlap between the task capabilities and the capabilities provided by the skill.

This will become useful when orchestration starts from a task instead of only from project inspection.

## Why candidates are separate

External discovery and approval are separate concerns.

~~~text
detected technology
      ↓
matching external skill
      ↓
candidate
      ↓
engineering review
      ↓
enabled
~~~

This prevents third-party or generated guidance from becoming trusted automatically.

## Registry loading

The implementation also introduces a small registry loader for `registry/skills.yaml`.

The loader validates basic structural invariants such as:

- positive schema version;
- skill IDs are present;
- map key and skill ID agree.

Full JSON Schema validation can be added as a later quality gate.

## Package locations

~~~text
core/registry
core/resolver
~~~

## What this step does not do

The resolver does not yet:

- download external `SKILL.md` content;
- enable candidate skills;
- generate `.ai/project.yaml`;
- choose an AI model;
- execute an agent;
- ask AI to resolve ambiguous technologies.

Those responsibilities remain separate.
