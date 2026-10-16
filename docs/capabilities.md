# Capability Contract

## What a capability is

A capability is a normalized description of something the system can do or needs to do.

It is intentionally independent from:

- a specific agent;
- a specific programming language;
- a specific AI model;
- a specific IDE or runtime.

This lets the orchestrator reason about required work before choosing who performs it.

Example:

```text
User request
  ↓
required capabilities
  ↓
candidate agents
  ↓
compatible skills
  ↓
available tools
```

## Example

A request such as:

> Add a Spring Boot endpoint to query card debt.

may require:

```yaml
required_capabilities:
  - backend-development
  - api-development
  - automated-testing
```

The system can then resolve an agent and attach technology-specific skills such as Java 21 and Spring Boot 3.

## Contract

Each capability has:

- `id`: stable identifier in kebab-case;
- `description`: what the capability means;
- `category`: broad classification;
- `aliases`: optional terms that help normalize requests;
- `status`: whether the capability is active or deprecated.

The JSON Schema is the source of truth for validation:

```text
schemas/capability.schema.json
```

The initial registry lives at:

```text
registry/capabilities.yaml
```

## Design rules

1. Capabilities describe work or expertise, not products or vendors.
2. Technology names should not become capabilities unless they represent a distinct type of work.
3. Agents provide capabilities.
4. Skills strengthen capabilities with domain or technology knowledge.
5. Projects require capabilities.
6. The orchestrator resolves capabilities before selecting an agent.
7. Capability IDs are stable once published. Rename by deprecating the old ID rather than silently changing it.

## Examples

Good capability IDs:

```text
backend-development
api-development
code-review
security-review
automated-testing
technical-research
```

Avoid:

```text
spring-agent
claude-code
cursor
java-21
postgresql
```

Those belong to agents, integrations, technologies or skills instead.
