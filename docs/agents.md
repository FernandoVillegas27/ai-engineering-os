# Agent Contract

## What an agent is

An agent is a stable engineering role.

It is not a model, an IDE, a prompt file or a specific vendor integration.

Examples:

- `backend-engineer`
- `frontend-engineer`
- `architect`
- `qa-engineer`
- `reviewer`

The same agent role should be usable from Cursor, Claude Code, Codex, OpenCode or another supported runtime.

## Why roles instead of technology-specific agents

The platform should not need a separate agent for every framework.

We want:

```text
backend-engineer
  + spring-boot-3 skill
  + java-21 skill
  + project context
  + available tools
  = Spring Boot expert for this task
```

and later:

```text
backend-engineer
  + django-drf skill
  + python skill
  + project context
  + available tools
  = Django expert for another task
```

This keeps the agent layer small and reusable.

## Agent fields

Each agent defines:

- `id`: stable machine-readable identifier;
- `role`: semantic engineering role;
- `mission`: the responsibility of the role;
- `capabilities`: work the agent is able to perform;
- `skill_domains`: categories of skills that are relevant to the role;
- `tools.required`: tools that must exist before the agent can operate;
- `tools.optional`: tools that improve the agent but are not mandatory;
- `permissions`: explicit safety boundaries;
- `status`: lifecycle state.

The validation source of truth is:

```text
schemas/agent.schema.json
```

The initial registry is:

```text
registry/agents.yaml
```

## Initial roles

The first version defines eight role-based agents:

1. `orchestrator`
2. `architect`
3. `researcher`
4. `frontend-engineer`
5. `backend-engineer`
6. `database-engineer`
7. `qa-engineer`
8. `reviewer`

These are roles, not independent AI products.

## Relationship with capabilities

Capabilities answer:

> What kind of work is required?

Agents answer:

> Which role is suitable for that work?

Example:

```text
Task: Add a backend endpoint and tests

Required capabilities:
  backend-development
  api-development
  automated-testing

Candidate:
  backend-engineer

Relevant skills:
  java-21
  spring-boot-3
```

## Relationship with skills

Agents should not contain framework-specific knowledge by default.

Technology knowledge is attached dynamically through skills.

This prevents duplication and allows the same role to work across different stacks.

## Permissions

Permissions are part of the contract instead of informal prompt instructions.

For example, the `reviewer` can inspect source code but cannot commit or push changes.

The `backend-engineer` can modify source code and tests, while commit and push operations require explicit approval.

Secrets and production access are denied by default.

## Design rules

1. Agents are role-based and technology-agnostic.
2. Agent IDs are stable.
3. Capabilities describe what the role can do.
4. Skills provide technology or domain specialization.
5. Tools describe what the role may use.
6. Permissions are explicit and deny sensitive access by default.
7. Models and IDEs do not belong in the agent definition.
8. An agent should not claim capabilities that are not present in the capability registry.
