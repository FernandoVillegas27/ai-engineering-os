# AI Classification Fallback

## Purpose

The deterministic task classifier remains the first path.

Only ambiguous requests may be escalated to an AI model provider.

~~~text
task
 ↓
deterministic classifier
 ├─ high confidence → planner
 └─ ambiguous
      ↓
   model provider
      ↓
   strict validation
      ↓
   planner
~~~

## Provider independence

AI Engineering OS does not bind classification to OpenAI, Anthropic, Cursor, Codex or any other runtime.

The core contract is:

~~~go
type Provider interface {
    ID() string
    Classify(ctx context.Context, request ClassificationRequest) (ClassificationResponse, error)
}
~~~

Concrete providers are adapters.

## Constrained request

A model provider receives:

- the original task;
- optional project context;
- the exact allowed execution intents;
- the exact active capability IDs.

The model is not asked to invent a taxonomy.

## Response contract

A provider can either return a classification:

~~~yaml
can_classify: true
intent: implement
capabilities:
  - frontend-development
rationale: project context indicates a UI change
~~~

or explicitly request clarification:

~~~yaml
can_classify: false
clarification_question: "¿Qué parte querés mejorar: frontend, backend o base de datos?"
~~~

## Validation

Every AI response is validated before it can reach orchestration.

The resolver rejects:

- unknown intents;
- unknown capabilities;
- disabled/deprecated capabilities;
- an empty capability set;
- a refusal to classify without a clarification question.

The model output is data, not authority.

## Deterministic fast path

If the deterministic classifier already resolves the request, the model provider is never called.

This keeps common tasks fast, cheap and reproducible.

## Ambiguity policy

AI fallback is allowed to say it still does not know.

For genuinely underspecified requests, asking one focused clarification question is better than fabricating a routing decision.

## Current scope

This step defines:

- the model-provider contract;
- AI fallback orchestration;
- response validation;
- clarification handling;
- provider-independent tests.

It does not yet connect a concrete model provider.

The next adapter can target one or more providers without changing the classifier or orchestration planner.
