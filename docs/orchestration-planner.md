# Deterministic Orchestration Planner

## Purpose

The first orchestration layer converts explicit required capabilities into a deterministic execution plan.

It does not ask an AI model to choose an agent.

~~~text
capabilities
    ↓
active agent coverage
    ↓
minimum useful worker set
    ↓
project-aware skill resolution
    ↓
logical tool requirements
    ↓
execution plan
~~~

## Why capabilities are explicit first

Natural-language task classification is probabilistic.

Agent and skill selection should remain reproducible after the required capabilities are known.

The initial boundary is therefore:

~~~text
user intent
   ↓
future capability classifier
   ↓
DETERMINISTIC BOUNDARY
   ↓
orchestration planner
~~~

This keeps the routing core testable and provider-independent.

## Agent selection

The planner uses deterministic greedy set coverage.

Rules:

1. only active agents are candidates;
2. the `orchestrator` role is not selected as an implementation worker;
3. the agent covering the most still-uncovered capabilities wins;
4. ties are resolved by stable agent ID order;
5. selection repeats until all capabilities are covered or no active agent can cover the remainder;
6. uncovered capabilities are returned explicitly.

The planner does not invent a missing specialist.

## Skills

For every selected worker, the existing Skill Resolver runs again with:

- project technology detection;
- the selected agent ID;
- the capabilities assigned to that agent.

Enabled skills remain separate from candidate skills.

Candidate skills are not silently promoted to trusted/enabled skills.

## Tools

Tool requirements combine:

- agent required tools;
- agent optional tools;
- required tools from matched skills;
- recommended tools from matched skills.

Only tool IDs that exist and are enabled in the Tool Registry are emitted.

At this stage this means logical platform availability, not a guarantee that the runtime adapter is currently connected.

Runtime availability remains the responsibility of provider/tool health checks such as `ai doctor`.

## CLI

Example:

~~~powershell
go run ./cmd/ai plan . --capability backend-development --capability api-development
~~~

Expected shape:

~~~yaml
requested_capabilities:
  - api-development
  - backend-development
agents:
  - agent: backend-engineer
    capabilities:
      - api-development
      - backend-development
    skills:
      candidates:
        - spring-boot-3
    tools:
      required:
        - filesystem
      recommended:
        - context7
        - engram
uncovered_capabilities: []
~~~

## Current boundary

This step does not:

- infer capabilities from natural language;
- launch Cursor subagents;
- execute tools;
- mutate source code;
- choose an LLM/model;
- promote candidate skills;
- authorize Git or production actions.

Those are separate layers.
