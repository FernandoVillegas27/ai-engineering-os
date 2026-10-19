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

The planner uses deterministic greedy set coverage with intent-aware ranking.

Rules:

1. only active agents are candidates;
2. the `orchestrator` role is never selected as a worker;
3. agents must be eligible for the requested execution intent;
4. the agent covering the most still-uncovered capabilities wins first;
5. ties are ranked by intent suitability and permissions;
6. remaining ties prefer the more specialized agent;
7. stable agent ID is the final tie-breaker;
8. selection repeats until all capabilities are covered or no eligible active agent can cover the remainder;
9. uncovered capabilities are returned explicitly.

### Execution intents

Supported initial intents are:

- `implement` — requires a worker with source-code or test write permission;
- `design` — favors the architect for architecture-oriented work;
- `research` — favors the researcher;
- `review` — favors independent reviewers and non-mutating Git permissions;
- `verify` — favors QA/test-oriented workers.

The default is `implement`.

This prevents a read-oriented agent from winning implementation work only because its ID sorts first.

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
go run ./cmd/ai plan . --intent implement --capability backend-development --capability api-development
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


## Intent example

The same capability may route differently depending on intent:

~~~text
database-design + implement
→ database-engineer

database-design + design
→ architect

code-review + review
→ reviewer
~~~

This is intentional. Capability answers **what work is needed**; intent answers **what kind of action is being performed**.
