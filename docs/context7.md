# Context7 Integration

## Purpose

Context7 is the initial documentation provider for AI Engineering OS.

Its role is narrow and intentional:

> Retrieve current, version-aware documentation and code examples for external libraries, frameworks, SDKs, APIs, CLI tools and cloud services.

It is not a general reasoning engine and it is not the default tool for business-logic debugging or code review.

## Architecture

~~~text
core/documentation
      ↓ provider contract
integrations/context7
      ↓
Context7 MCP
      ↓
current library/framework documentation
~~~

This keeps the core independent from Context7 while still giving agents a standard documentation provider.

## Official MCP operations

The Context7 MCP server currently exposes two tools:

~~~text
resolve-library-id
query-docs
~~~

The expected flow is:

~~~text
library name + task
      ↓
resolve-library-id
      ↓
Context7 library ID
      ↓
query-docs
      ↓
current documentation
~~~

If an exact Context7 library ID is already known, the first step may be skipped.

## When agents should use Context7

Use Context7 for:

- library or framework API documentation;
- version-sensitive syntax;
- migration guidance;
- package configuration;
- SDK usage;
- CLI usage;
- current code examples from external dependencies.

Do not use Context7 as the primary tool for:

- project-specific business logic;
- generic refactoring;
- code review;
- repository architecture discovery;
- debugging code that depends mainly on local application behavior.

Those responsibilities belong to project inspection, code-intelligence tools, agents and tests.

## Local MCP configuration

AI Engineering OS models the local MCP server as:

~~~json
{
  "command": "npx",
  "args": [
    "-y",
    "@upstash/context7-mcp@4.4.2"
  ]
}
~~~

The package version is pinned in the adapter so platform behavior does not silently change when upstream publishes a new release.

Updating the pin should be an explicit platform change.

## Availability

The initial deterministic status probe checks whether `npx` exists and can report its version.

This proves that the local launcher required by the MCP configuration is ready.

It intentionally does not launch Context7 during a health check because doing so through `npx -y` could download or mutate the local package cache.

Actual Context7 server connectivity is verified by the MCP host when it starts the configured server.

## Existing Cursor configuration

Cursor may use either a local Context7 MCP server or the official remote HTTP server.

AI Engineering OS does not depend on Cursor's config format.

The adapter describes the provider independently, and future runtime adapters can project it into Cursor, Codex, Claude Code, OpenCode or other MCP hosts.

## Authentication

Context7 supports API-key-backed local MCP and authenticated remote HTTP configurations.

Credentials must remain outside the registry and source code.

AI Engineering OS must never store Context7 API keys in:

- `registry/tools.yaml`;
- project YAML;
- skill metadata;
- committed runtime configuration.

Runtime adapters should consume credentials from the runtime's secure configuration or environment.

## Current boundary

This step does not yet:

- call Context7 tools directly from the Go core;
- modify Cursor MCP configuration;
- create or manage Context7 API keys;
- install the Context7 package globally;
- add model routing;
- execute agents.

The MCP host remains responsible for invoking the actual Context7 tools.
