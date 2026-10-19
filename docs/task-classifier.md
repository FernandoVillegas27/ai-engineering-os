# Task Classifier

## Purpose

The task classifier converts natural-language requests into the normalized inputs required by the deterministic orchestration planner:

~~~text
natural-language task
      ↓
intent + capabilities
      ↓
deterministic planner
      ↓
agents + skills + tools
~~~

## Deterministic first

The classifier first resolves high-confidence language using:

- capability IDs;
- capability aliases from `registry/capabilities.yaml`;
- a small set of curated domain phrases;
- explicit action verbs for execution intent.

This allows common requests to be resolved without an LLM.

Examples:

~~~text
"Implementá un endpoint REST para productos en el backend"

intent:
  implement

capabilities:
  - api-development
  - backend-development
~~~

~~~text
"Revisá este cambio de base de datos"

intent:
  review

capabilities:
  - code-review
  - database-design
~~~

## Ambiguity boundary

The deterministic classifier must not guess when the task does not contain enough evidence.

Example:

~~~text
"Necesito mejorar esto"
~~~

returns:

~~~text
confidence: ambiguous
needs_ai: true
~~~

The future AI classifier will receive:

- the original task;
- the canonical capability registry;
- the supported execution intents;
- project context.

Its result must still be validated against the same canonical registries before orchestration.

## Supported intents

- `implement`
- `design`
- `research`
- `review`
- `verify`

## CLI

Classify only:

~~~powershell
go run ./cmd/ai classify "Revisá este cambio de base de datos"
~~~

Plan directly from natural language:

~~~powershell
go run ./cmd/ai plan . --task "Implementá un endpoint REST para productos en el backend"
~~~

When deterministic classification is ambiguous, `ai plan --task` stops instead of silently choosing an agent.

## Safety boundary

Classification does not authorize execution.

Even a high-confidence result only produces planning inputs.

~~~text
classification
≠ authorization
≠ execution
~~~

Agent permissions, tool policy and runtime execution remain separate layers.


## Language variants

Deterministic classification includes curated language variants for common engineering requests when they are unambiguous.

For example:

~~~text
"Quiero que revises si la forma en que manejamos los permisos puede tener problemas"
~~~

resolves deterministically to:

~~~yaml
intent: review
capabilities:
  - code-review
  - security-review
~~~

This keeps clear requests on the deterministic fast path instead of escalating them to a model provider.
