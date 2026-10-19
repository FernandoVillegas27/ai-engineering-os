# Codex CLI Model Provider

## Purpose

Codex CLI is the first concrete implementation of the provider-neutral model classification contract.

The core remains provider-independent.

~~~text
core/modelprovider
      ↓
integrations/commandmodel
      ↓
integrations/codex
      ↓
codex exec
~~~

## Command provider

The command provider owns reusable process concerns:

- binary discovery through PATH;
- temporary isolated workspace creation;
- stdin delivery;
- stdout/stderr capture;
- process execution through context;
- cleanup;
- adapter-driven response decoding.

It does not know Codex prompts, flags or schemas.

## Codex adapter

The Codex adapter uses non-interactive `codex exec`.

The adapter runs in an isolated temporary directory, so it explicitly skips the repository trust check.

Classification runs with:

~~~text
--skip-git-repo-check
--sandbox read-only
-c 'approval_policy="never"'
--output-schema <temporary-schema>
--output-last-message <temporary-result>
-
~~~

The final `-` tells Codex to read the classification prompt from stdin.

The classification task must not require filesystem writes, shell execution or approval.

## Structured output

The temporary JSON Schema uses only the structured-output subset supported by Codex and constrains:

- `intent` to the exact allowed intent enum;
- every element in `capabilities` to the exact allowed capability enum;
- `can_classify` to boolean;
- clarification and rationale to strings.

The existing AI fallback validator still validates the decoded response afterward.

Therefore there are two boundaries:

~~~text
Codex structured-output schema
          ↓
core canonical validation
          ↓
orchestration
~~~

## Authentication

The adapter reuses the existing Codex CLI installation and authentication.

AI Engineering OS does not store OpenAI credentials.

If Codex is unavailable or unauthenticated, the provider returns an operational error rather than falling back to an unvalidated answer.

## Explicit AI usage

The first CLI integration keeps AI escalation explicit.

~~~powershell
go run ./cmd/ai classify --ai "Necesito mejorar esto"
~~~

Without `--ai`, the deterministic classifier behaves exactly as before.

This avoids silently consuming model usage for an ambiguous request.

## Project context

The provider contract supports project context, but this adapter does not inspect arbitrary files on its own.

The caller decides which normalized project context is safe and useful to send to the classifier.

## Safety

Classification is analysis-only.

The Codex process is launched read-only and the result is treated as untrusted structured data until canonical validation succeeds.


## Structured-output compatibility

Codex structured output accepts only a subset of JSON Schema keywords.

The adapter intentionally does not use `uniqueItems` for capabilities. Duplicate capability IDs, if any, are normalized by the core validator before orchestration.
