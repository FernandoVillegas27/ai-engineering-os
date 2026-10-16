# Tool Contract

## What a tool is

A tool is something an agent can use to observe, modify, execute or retrieve information from the environment.

Examples include:

- `filesystem`
- `github`
- `context7`
- `codegraph`
- `engram`
- `playwright`

A tool is not an agent and it is not a skill.

The distinction is:

~~~text
Agent = who performs the work
Skill = specialized knowledge
Tool = what the agent can use
~~~

## Why tools need their own contract

Different runtimes expose tools differently.

One environment may provide GitHub through a connector, another through MCP, and another through a CLI.

The core platform should reason about the logical tool rather than hard-coding a specific transport.

For example:

~~~text
Agent requests: github
        ↓
Tool Registry
        ↓
available implementation
        ↓
GitHub connector / MCP / CLI
~~~

This is part of the portability goal of AI Engineering OS.

## Interface

The `interface.type` field describes how the current implementation is exposed.

Supported initial values are:

- `native`
- `mcp`
- `connector`
- `cli`
- `api`

This is runtime metadata, not the identity of the tool itself.

The logical tool remains `github`, `context7`, `engram`, and so on.

## Operations

Each tool declares the operations it can provide.

For example, GitHub may expose:

~~~yaml
operations:
  - read-repository
  - create-branch
  - create-commit
  - create-pull-request
~~~

This will later allow the resolver to check whether a tool is merely present or actually supports the operation required by a task.

## Access

The `access` field describes broad effects:

- `read`
- `write`
- `execute`
- `network`

This does not replace agent permissions.

Both layers matter.

An agent may be allowed to use GitHub generally while still being denied permission to push code.

## Risk

Tools have a coarse risk level:

- `low`
- `medium`
- `high`

Risk is metadata for future policy and quality gates.

A read-only documentation lookup is different from an operation that can modify a remote repository.

## Approval

`requires_approval` indicates that use of a tool may require explicit user approval for sensitive operations.

This is intentionally separate from agent permissions.

The final decision can later combine:

~~~text
agent permission
+ tool risk
+ requested operation
+ project policy
= allow / ask / deny
~~~

## Availability

A tool definition may expose an `availability_check`.

The runtime should determine availability deterministically whenever possible.

Example:

~~~text
backend-engineer requires filesystem

filesystem available? yes
→ agent can run

filesystem available? no
→ agent cannot run
~~~

The platform should not ask an AI model to guess whether a tool exists.

## Initial registry

The first registry contains:

- `filesystem`
- `github`
- `context7`
- `codegraph`
- `engram`
- `playwright`

These entries describe logical tools and their current preferred interfaces.

They do not permanently bind the platform to those implementations.

## Tools and agents

Agents declare required and optional tools.

Example:

~~~yaml
backend-engineer:
  tools:
    required:
      - filesystem
    optional:
      - github
      - context7
      - codegraph
      - engram
~~~

Before execution, the future Tool Resolver will compare the agent requirements with the tools actually available in the runtime.

## Tools and skills

Skills may also require or recommend tools.

For example, a framework skill may recommend `context7` so the agent can confirm current documentation before implementing version-sensitive code.

## MCP is not the tool model

MCP is one way to expose tools.

AI Engineering OS should not model every tool as an MCP server.

A tool may come from a connector, local runtime, CLI or API.

This keeps the core independent from one integration protocol.

## Validation

The machine-readable contract is:

`schemas/tool.schema.json`

The initial registry is:

`registry/tools.yaml`

## Design rules

1. Tools represent usable external or local capabilities.
2. Tool identity is independent from transport or protocol.
3. MCP is an interface option, not the platform architecture.
4. Availability should be checked deterministically.
5. Operations must be explicit enough to support capability checks.
6. Tool risk and agent permissions are separate inputs to authorization.
7. Sensitive operations should be approval-aware.
8. Agents and skills reference tool IDs rather than provider-specific implementations.
