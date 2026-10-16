# Architecture

## Principle

AI Engineering OS must remain independent from any specific AI provider, IDE or coding agent.

External systems such as Gentle-AI, Engram, Cursor, Claude Code, Codex and OpenCode are integrations of the platform, not the platform itself.

## Core concepts

The platform will be built around:

- Agent
- Skill
- Capability
- Tool
- Project

## Core principle

Deterministic first, AI second.

Deterministic mechanisms should be used for:

- project detection
- registry lookup
- capability resolution
- tool discovery
- configuration validation

AI should be used for:

- reasoning
- planning
- ambiguous decisions
- implementation
- review
- research