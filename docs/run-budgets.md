# Coordinated Run Budgets

## Purpose

Unattended coordinated runs need hard usage ceilings that are independent from the selected model provider.

`ai run execute` now creates one shared run budget before AI classification, memory recall, or runtime execution.

## Defaults

~~~text
--max-invocations 64
--max-runtime 30m
~~~

Explicit bounds:

- invocations: 1..10000;
- runtime: 1 second..24 hours.

## What consumes invocation budget

The same counter is shared by:

- AI task classification;
- project-memory status/recall/save calls;
- read-only agent execution;
- proposal generation;
- automatic provider fallback;
- automatic proposal escalation;
- isolated staging;
- independent review;
- Quality correction.

Every concrete provider attempt is charged before it is invoked.

For `--runtime auto`, if the primary provider fails and the fallback provider is attempted, both attempts consume budget.

If a proposal is rejected for task drift and policy allows escalation, the escalated attempt also consumes budget.

Deterministic operations such as local verification, event projection, snapshot verification, integration materialization, and promotion validation do not consume provider invocation budget.

## Runtime deadline

A single deadline context spans the coordinated run.

It covers provider calls, memory operations, coordination, mutation phases, Quality, and checkpoints that honor the run context.

Provider-specific shorter deadlines still apply. The earliest deadline wins.

When the global deadline is reached:

- active provider calls receive context cancellation;
- read-only coordination terminates through its existing cancellation path;
- mutating runs use the existing abort/cancel path;
- no additional provider invocation is permitted.

## Exhaustion behavior

Invocation exhaustion returns a typed `runbudget.Error` with:

~~~text
kind=invocation-limit
operation=<provider operation>
used=<count>
limit=<count>
~~~

Duration exhaustion reports:

~~~text
kind=runtime-limit
~~~

The check happens **before** invoking the provider, so exceeding quota does not make one extra model/tool call.

## Provider independence

Budgets wrap core interfaces rather than Cursor/Codex implementations.

The policy therefore remains valid when providers change and does not pin a model to any agent role.

## Existing complementary limits

This run-level budget composes with existing safety limits:

- Quality correction attempts remain bounded;
- provider/tool operations have their own deadlines;
- tool-health never retries automatically;
- scheduler concurrency is bounded by `--max-concurrency`;
- promotion remains approval-gated.
