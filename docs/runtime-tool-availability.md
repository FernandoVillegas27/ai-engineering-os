# Runtime tool availability and binding

AI Engineering OS separates three questions that must never be conflated:

1. **Is the tool declared and enabled in the registry?**
2. **Is the provider currently healthy/reachable?**
3. **Is that tool explicitly bound to the selected execution runtime?**

Only the combination of all required evidence can make an external tool executable by a work unit.

## CLI

~~~text
ai tools status
ai tools status --json
~~~

The command reads the canonical Tool Registry and produces a local runtime availability snapshot.

Current evidence sources:

- native tools: locally available by construction when enabled;
- Engram: provider health is probed through the existing Engram adapter;
- Context7: local MCP launcher health can be probed, but remote MCP-host connectivity remains unverified;
- GitHub connector: unknown until a connector/runtime binding can be verified;
- Codegraph MCP: unknown until a runtime adapter can verify it;
- Playwright MCP: unknown until a runtime adapter can verify it.

The human output shows both provider state and:

~~~text
bound=yes|no
~~~

A healthy provider with `bound=no` is **not** an executable agent capability.

## Runtime work units

For real `ai run execute` runs, the declarative orchestration plan is narrowed before work-unit creation.

The narrowing operation can only:

- keep a required tool that has sufficient evidence;
- remove an optional/recommended tool;
- block a run when a required external tool lacks verified availability or binding.

It can never add a tool that was not already present in the declarative plan.

Current Codex/Cursor work-unit runtimes do not expose a generic MCP/connector binding owned by AI Engineering OS.

Therefore external logical tools are currently removed from executable work units unless a future runtime adapter explicitly marks them bound.

This is intentional. The prompts already describe logical tools as planning metadata and must not imply capabilities that the runtime cannot actually provide.

## Dry-run behavior

Dry-run remains declarative and performs no external health probes.

This keeps dry-run:

- deterministic;
- fast;
- free of external side effects.

A dry-run may show registry-enabled optional tools because it describes the declarative plan, not proven runtime execution availability.

## Required external tools

A required external tool fails closed when:

- no runtime evidence exists;
- evidence is unknown;
- the provider is unavailable/unhealthy;
- the provider is healthy but the tool is not bound to the selected runtime.

Example:

~~~text
required external tool "github" has no verified runtime availability evidence
~~~

or:

~~~text
required external tool "engram" is healthy but not bound to the selected execution runtime
~~~

## Optional external tools

Optional/recommended external tools are omitted unless they are both:

- verified available;
- explicitly runtime-bound.

Unknown does not mean unavailable globally. It means AI Engineering OS cannot prove the capability for this execution and therefore will not advertise it to the agent.

## Permission boundary

Availability and binding are filters, never permission grants.

They cannot:

- change an agent permission envelope;
- authorize source mutation;
- authorize secrets/production access;
- approve promotion;
- turn a missing registry dependency into an allowed tool;
- substitute another provider automatically.

The direction is always:

~~~text
registry plan
  -> permission boundary
  -> provider health evidence
  -> explicit runtime binding
  -> executable subset
~~~
