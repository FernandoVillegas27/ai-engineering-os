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


## Context7 AI Engineering OS-owned binding

Context7 now has one concrete execution binding that does not depend on model-runtime MCP configuration.

For real runs whose selected plan declares `context7`:

1. AI Engineering OS derives up to four public library hints only from deterministic project technology detection.
2. It never sends the user's task text, repository source, prompts, file paths, or business context to Context7.
3. It runs the pinned official CLI package:
   `ctx7@0.10.0 search <generic-doc-query> --library ... --json`.
4. The child process runs with a controlled environment and `CTX7_TELEMETRY_DISABLED=1`.
5. `CONTEXT7_API_KEY` is not implicitly forwarded.
6. The provider output must be valid JSON and is bounded before it can enter a work unit.
7. Returned documentation is prefixed as **untrusted external evidence**.
8. Only work units whose post-availability tool plan still contains `context7` receive that context.

A successful prefetch produces runtime availability evidence:

~~~text
state=available
healthy=true
runtime_bound=true
~~~

A provider/network/rate-limit/protocol failure produces unavailable evidence. Because Context7 is currently optional in the built-in agents, the planner removes it rather than failing an otherwise valid run.

If a future agent/skill makes Context7 required, the existing fail-closed required-tool rule applies and the run cannot proceed unless this binding succeeds.

### Privacy boundary

The outbound query is intentionally generic:

~~~text
Current API usage, compatibility notes, and recommended patterns for the detected public project libraries.
~~~

Only detected public technology names such as Angular, React, Next.js, Spring Boot, Django, pytest, Java, Go, Python, or TypeScript may be used as library hints.

The adapter does not give Cursor, Codex, or another model runtime direct network/MCP access. The model sees only the bounded documentation evidence returned by the AI Engineering OS-owned adapter.

### Why not Cursor global MCP

Cursor can merge global and project MCP configuration. AI Engineering OS does not use blanket MCP approval as the Context7 binding because that could expose unrelated user-configured servers.

Provider auto-control files are also removed from sanitized runtime views as documented in `runtime-instruction-isolation.md`.


## Operation-scoped runtime binding

Runtime binding evidence is now operation-scoped.

Each availability record can expose:

~~~json
{
  "tool_id": "context7",
  "state": "available",
  "healthy": true,
  "runtime_bound": true,
  "bound_operations": ["search-docs"]
}
~~~

For an external tool, `runtime_bound=true` is not valid evidence by itself. A valid bound record must contain at least one exact operation that is also declared by that tool in the canonical Tool Registry.

Validation fails closed when:

- a bound tool has no bound operations;
- operations exist while `runtime_bound=false`;
- a bound operation is not declared in the Tool Registry;
- a tool is marked bound while its availability state is not `available`;
- duplicate/empty operation evidence is supplied.

Native tools automatically bind their declared registry operations.

### Context7

The AI-OS-owned Context7 prefetch binds only:

~~~text
search-docs
~~~

It does not claim that the runtime exposes Context7's MCP-native `resolve-library-id` or `query-docs` operations directly to the model.

### Why this matters for higher-risk providers

A future GitHub adapter may prove read-only operations such as:

~~~text
read-repo
read-issues
read-pull-requests
~~~

without proving or granting:

~~~text
create-branch
create-commit
create-pull-request
~~~

Operation evidence remains subordinate to the agent permission envelope, run mode, mutation capability, and explicit promotion/approval boundaries.


## GitHub AI Engineering OS-owned read binding

GitHub now has an AI Engineering OS-owned read-only runtime binding based on the local GitHub CLI.

The adapter never grants general GitHub authority to the model runtime. Instead AI Engineering OS executes a fixed allowlist of non-mutating commands itself and injects only bounded JSON evidence.

Current bindable operations:

~~~text
read-repository
read-issues
read-pull-requests
~~~

Fixed command shapes:

~~~text
gh auth status
gh repo view --json nameWithOwner,url,defaultBranchRef,isPrivate
gh issue list --state open --limit 20 --json number,title,state,updatedAt,url,labels
gh pr list --state open --limit 20 --json number,title,state,isDraft,updatedAt,url,headRefName,baseRefName
~~~

The adapter does not construct or expose commands for:

~~~text
create-branch
create-commit
create-pull-request
review-pull-request
merge
edit
delete
~~~

### Credential boundary

The trusted adapter may forward GitHub-specific authentication variables required by `gh`:

- `GH_TOKEN`;
- `GITHUB_TOKEN`;
- `GH_HOST`.

Unrelated environment secrets are not forwarded.

The token itself is never included in model context. Diagnostic/provider errors pass through the shared redaction boundary.

The process is explicitly non-interactive:

~~~text
GH_PROMPT_DISABLED=1
GH_PAGER=cat
NO_COLOR=1
~~~

### Partial binding

Read operations are independent.

For example, if repository metadata and pull requests are readable but issue listing is forbidden, the evidence may become:

~~~json
{
  "runtime_bound": true,
  "bound_operations": [
    "read-pull-requests",
    "read-repository"
  ]
}
~~~

The unavailable operation is not inferred from provider health.

If authentication fails, `gh` is absent, or no read operation returns valid JSON, GitHub remains unavailable/unbound and optional GitHub access is removed from the plan.

### Operation-scoped approvals

GitHub retains high-risk mutation/review operations in the canonical Tool Registry, but approval is scoped to:

- `create-branch`;
- `create-commit`;
- `create-pull-request`;
- `review-pull-request`.

The read operations do not require that mutation approval.

Runtime availability validation rejects any approval-required operation that appears bound without a future explicit approval-evidence layer. Therefore an authenticated high-privilege GitHub token does not make write operations available merely because read-only binding succeeded.

### Evidence trust

Repository metadata, issue titles, PR titles, labels and other GitHub fields are labeled **untrusted external data** before entering work-unit context.

They cannot change:

- task;
- permissions;
- system/safety rules;
- runtime/model policy;
- promotion approval.
