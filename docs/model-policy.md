# Model policy

AI Engineering OS can keep Cursor's economical automatic model selection while still escalating to fixed models when deterministic evidence shows that more capability is needed.

## Policies

`--model-policy auto` is the default.

- `auto`: start with Cursor's own automatic model selection. If a proposal is rejected by the deterministic task-alignment guardrail, retry once with the balanced engineering model. The stronger model is then kept for the rest of that staged flow.
- `economy`: always leave model selection to Cursor. No automatic model escalation is performed.
- `balanced`: use a fixed stronger engineering model for proposal, stage and correction, with an independent review model.
- `deep`: use a higher-reasoning fixed model for difficult work and an independent high-reasoning review model.

Current routing:

| Policy | Proposal / Stage / Correction | Review |
| --- | --- | --- |
| auto | Cursor Auto; on task drift → `gpt-5.6-sol-high` | Cursor Auto; if the flow escalated → `claude-sonnet-5-thinking-high` |
| economy | Cursor Auto | Cursor Auto |
| balanced | `gpt-5.6-sol-high` | `claude-sonnet-5-thinking-high` |
| deep | `gpt-5.6-sol-xhigh` | `claude-opus-5-thinking-high` |

The exact model IDs are policy configuration, not core architecture. They can be changed later without changing the runtime contracts.

## Explicit override

Use `--model <id>` to force a Cursor model for the flow.

Example:

~~~powershell
go run ./cmd/ai stage . `
  --task "Implementar validación" `
  --runtime cursor `
  --model gpt-5.6-sol-high
~~~

An explicit model override disables automatic proposal-model escalation because the user already chose the model.

## Examples

Economical default:

~~~powershell
go run ./cmd/ai stage . `
  --task "Implementar validación" `
  --runtime cursor
~~~

Never auto-escalate:

~~~powershell
go run ./cmd/ai stage . `
  --task "Implementar validación" `
  --runtime cursor `
  --model-policy economy
~~~

Start strong:

~~~powershell
go run ./cmd/ai stage . `
  --task "Refactorizar una pieza compleja" `
  --runtime cursor `
  --model-policy balanced
~~~

High-reasoning route:

~~~powershell
go run ./cmd/ai stage . `
  --task "Rediseñar una parte crítica con varias restricciones" `
  --runtime cursor `
  --model-policy deep
~~~

## Runtime auto

When `--runtime auto` is used, provider fallback remains independent from model policy:

~~~text
provider selection
Codex
  ↓ provider unavailable
Cursor

Cursor model selection
policy auto/economy/balanced/deep
~~~

Model policy never expands permissions. The same isolated workspace, declared-file scope, deterministic verification and review gates still apply.
