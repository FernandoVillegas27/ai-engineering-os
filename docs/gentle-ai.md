# Gentle-AI Adapter

## Purpose

Gentle-AI is an integration layer for projecting AI Engineering OS into supported coding-agent runtimes.

It is not the core orchestrator.

AI Engineering OS owns:

- agent roles;
- capabilities;
- skill metadata and trust;
- tool metadata;
- project context;
- provider selection;
- future orchestration policy.

Gentle-AI can then configure compatible runtimes with managed guidance, skills, MCP configuration and review/workflow assets.

## Architecture

~~~text
AI Engineering OS
  Agent Registry
  Skill Registry
  Tool Registry
  Project Context
        ↓
core/projection
        ↓
integrations/gentleai
        ↓
Gentle-AI
        ↓
Cursor / Claude Code / Codex / OpenCode / ...
~~~

This preserves vendor and runtime independence.

## Current stable line

The adapter currently pins the known stable Gentle-AI release:

~~~text
v4.2.0
~~~

On Windows, the source installation command is modeled as:

~~~powershell
go install github.com/gentleman-programming/gentle-ai/v4/cmd/gentle-ai@v4.2.0
~~~

Updating this version is an explicit platform change.

## Discovery

The adapter can deterministically check:

~~~text
gentle-ai version
gentle-ai doctor
~~~

This is enough for `ai doctor` and future runtime diagnostics to determine whether Gentle-AI is installed and healthy.

## Preview first

This step intentionally exposes only non-mutating plans.

For Cursor:

~~~text
gentle-ai install --dry-run --agent cursor --preset minimal
~~~

and, after installation:

~~~text
gentle-ai sync --dry-run --agent cursor
~~~

The adapter returns these as projection plans with:

~~~yaml
mutates: false
~~~

Applying an install or sync remains a separate explicit action.

## Why minimal first

AI Engineering OS already owns Engram and Context7 integration decisions.

Using Gentle-AI's `minimal` preset first reduces the chance that an installer silently becomes the source of truth for components we already manage.

After previewing the exact plan, we can deliberately decide which Gentle-AI-managed assets we want for each runtime.

## Supported targets

The adapter recognizes the current Gentle-AI runtime IDs, including:

- `cursor`
- `claude-code`
- `codex`
- `opencode`
- `vscode-copilot`
- `gemini-cli`
- `windsurf`
- `antigravity`
- `kiro-ide`
- `qwen-code`
- `pi`

and the remaining supported integrations exposed by Gentle-AI.

## Projection vs orchestration

Gentle-AI provides its own ODD/RDD workflow assets and runtime-specific behavior.

AI Engineering OS should not automatically treat all of that behavior as authoritative.

The integration boundary is:

~~~text
our normalized intent
      ↓
projection adapter
      ↓
runtime-native configuration
~~~

Later milestones can decide which Gentle-AI workflow assets to reuse, override or map into our own multi-agent orchestration.

## Safety

The adapter does not currently:

- install Gentle-AI;
- run `gentle-ai install`;
- run `gentle-ai sync`;
- modify Cursor;
- overwrite our registries;
- enable RDD;
- install CodeGraph;
- change models.

It only discovers status and creates preview commands.

## Next step

After this adapter is merged, the local validation flow is:

~~~text
install pinned Gentle-AI binary
      ↓
ai gentle status
      ↓
ai gentle plan cursor
      ↓
run Gentle-AI dry-run
      ↓
inspect proposed changes
      ↓
apply only after approval
~~~
