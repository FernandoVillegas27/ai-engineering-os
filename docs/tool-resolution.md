# Tool resolution semantics

AI Engineering OS treats tool planning as a deterministic contract, not a suggestion that may silently disappear.

## Registry status vs runtime health

These concepts are deliberately separate:

- **registry status** answers whether a tool is declared and enabled for planning;
- **runtime health** answers whether an external provider is currently reachable/healthy;
- **permissions** answer whether the current work unit is allowed to use a capability.

A healthy provider never grants permission.

An enabled registry entry does not prove that an external service is currently reachable.

## Required tools

Required tools come from:

1. the selected agent's `tools.required`;
2. required tools of skills that are actually enabled for the work unit.

For every required tool:

- the tool must exist in `registry/tools.yaml`;
- its registry status must be `enabled`.

If either condition fails, orchestration stops before a work unit is created.

Example failure:

~~~text
agent "backend-engineer" requires tool "filesystem" but it is not present in the tool registry
~~~

or:

~~~text
agent "backend-engineer" requires tool "filesystem" but its registry status is "disabled"
~~~

A required dependency is never silently downgraded to optional.

## Optional / recommended tools

Optional agent tools and recommended skill tools are degradable planning capabilities.

If an optional/recommended tool:

- is missing from the registry; or
- is disabled;

it is omitted from the plan.

This does not make the work unit invalid because the agent or skill explicitly declared the tool optional/recommended.

## Candidate skills

Candidate skills do not contribute executable required tools.

Only a skill that is actually enabled for the selected work unit can add a required dependency.

This prevents discovery/recommendation metadata from accidentally expanding the execution surface.

## Deduplication

Required and recommended tools are normalized deterministically:

- empty values are ignored before resolution;
- duplicates are removed;
- output order is stable;
- recommended tools already present in the required set are removed.

## External providers

External MCP/connector entries additionally require provider and availability-check metadata at registry load time.

That metadata is declarative. Actual provider availability must be resolved separately through normalized health evidence.

The next runtime availability layer may:

- block a required external tool when it is known unavailable;
- omit an optional external tool when it is unavailable;
- expose typed health evidence.

It may **not**:

- grant access because a tool is healthy;
- bypass an agent permission envelope;
- silently substitute another provider;
- turn an optional connector into a required one;
- permit model-driven self-approval.
