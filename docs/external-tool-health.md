# External Tool and MCP Health Contract

## Purpose

External tools and MCP-backed providers must fail consistently.

AI Engineering OS treats external integrations as optional capabilities behind explicit permissions. A failed documentation, memory, projection, connector, or MCP provider must not silently expand permissions, block unrelated deterministic work, or expose provider-specific failure semantics to the core.

## Normalized health model

External adapters can expose:

- provider identity;
- availability;
- health/readiness;
- version;
- bounded diagnostic detail;
- normalized failure kind;
- retryability;
- probe duration.

Failure kinds:

- `unavailable`;
- `timeout`;
- `auth`;
- `permission`;
- `rate-limit`;
- `transport`;
- `protocol`;
- `unhealthy`;
- `unknown`.

Retryability is evidence only. The health layer does **not** automatically retry an operation.

## Deadlines

Defaults:

~~~text
health/status probe: 5 seconds
external operation: 30 seconds
~~~

Adapters may override those values explicitly for tests or specialized deployments.

A timeout cancels the provider command through context cancellation and reports:

~~~text
kind=timeout
retryable=true
~~~

There is no hidden retry loop.

## Current adapters

The common health semantics are used by:

- Engram;
- Context7 launcher health;
- Gentle-AI.

Engram Save/Recall also use bounded operation deadlines.

Context7 local health proves that the MCP launcher can be started. Remote MCP connectivity remains the responsibility of the active MCP host/session.

## Tool registry contract

External interfaces declared as:

- `mcp`;
- `connector`;

must include:

- an explicit provider;
- an `availability_check`.

All tools must declare:

- description;
- at least one operation;
- at least one supported access mode;
- risk: `low|medium|high`;
- status: `enabled|disabled`.

Supported access modes are:

- `read`;
- `write`;
- `execute`;
- `network`.

Malformed registry metadata fails closed while loading the registry.

## Doctor behavior

`ai doctor` consumes normalized health results.

External provider failures are diagnostic warnings, not environment-wide fatal errors. The output includes normalized failure kind and whether a retry may make sense.

This lets the rest of the deterministic system continue when an optional provider is unavailable while keeping the degradation visible.

## Permission boundary

Health and retryability never grant permissions.

The Tool Registry, agent permission envelope, mutation capability, promotion approval, and runtime sandbox continue to decide what a tool may do.

A provider becoming healthy cannot change those permissions automatically.
