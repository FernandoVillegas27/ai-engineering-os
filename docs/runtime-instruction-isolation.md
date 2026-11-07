# Runtime project-instruction isolation

## Purpose

AI Engineering OS treats repository content as untrusted engineering evidence.

Provider runtimes may auto-discover project files as instructions, skills, or tool configuration. Allowing those files into a sanitized runtime workspace would create a second policy channel outside:

- the AI Engineering OS task;
- platform/system safety rules;
- agent permission envelopes;
- the reviewed skill registry;
- explicit tool availability/binding.

The sanitized runtime view therefore removes provider auto-control files before a model runtime is launched.

## Excluded runtime-control paths

The default Safe View excludes the following directories anywhere in the project tree:

~~~text
.cursor/
.codex/
.claude/
.agents/
~~~

It also excludes these instruction basenames anywhere in the tree:

~~~text
AGENTS.md
AGENTS.override.md
CLAUDE.md
~~~

This boundary is separate from secret/sensitive-path filtering. A file can be non-secret and still be unsafe to auto-load as runtime policy.

## Consequences

Repository-owned instructions remain available to humans in the original repository, but they are not copied into the provider runtime's sanitized project view.

Project MCP configuration such as:

~~~text
.cursor/mcp.json
~~~

cannot silently add tools to a Cursor execution.

Project-local skills under:

~~~text
.agents/skills/
~~~

cannot bypass the AI Engineering OS reviewed/pinned skill registry.

## Tool binding

External tools are injected only by AI Engineering OS-owned binding logic after:

1. the tool is declared and enabled in the canonical registry;
2. provider availability/health evidence is collected;
3. the selected runtime explicitly verifies/binds that tool;
4. the agent permission envelope still permits the resulting operation.

Repository text cannot create a binding.

## Trust boundary

The exclusion does not mean repository documentation is discarded. Ordinary source, README files, architecture documentation, build files and other allowed project evidence remain visible in the Safe View.

The rule is narrowly targeted at files known to be auto-interpreted by model runtimes as control/configuration rather than ordinary project evidence.
