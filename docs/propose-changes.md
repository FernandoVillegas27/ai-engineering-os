# Propose Changes

## Purpose

`ai propose` is the bridge between read-only analysis and future controlled write execution.

It asks an approved runtime to inspect the sanitized project view and return a structured change proposal without applying any mutation.

## Command

~~~powershell
go run ./cmd/ai propose . --task "Add validation for negative product prices" --runtime codex
~~~

Use `--ai` only when deterministic task classification needs fallback.

## Output contract

A proposal contains:

- summary;
- affected files with action and rationale;
- unified diff text;
- risks with severity;
- verification steps.

Example shape:

~~~yaml
summary: Add domain validation for negative prices
files:
  - path: core/product/service.go
    action: modify
    rationale: reject invalid prices
patch: |
  --- a/core/product/service.go
  +++ b/core/product/service.go
  ...
risks:
  - severity: low
    detail: existing callers may now receive a validation error
verification:
  - go test ./...
~~~

## Safety boundary

Proposal generation is still read-only.

The Codex proposal runtime:

- validates the same read-only Work Unit permission envelope;
- runs only against the sanitized safe project view;
- uses the same isolated environment policy;
- runs with Codex `--sandbox read-only`;
- uses `approval_policy="never"`;
- is bounded by the same runtime timeout;
- returns the patch only as structured text.

The original repository is never the runtime working directory and no patch is applied.

## Non-goals

This step does not:

- edit source files;
- apply patches;
- run tests;
- create commits;
- push branches;
- access secrets;
- access production.

Those capabilities belong to later controlled-execution stages.
