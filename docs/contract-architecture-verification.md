# Deterministic Contract and Architecture Verification

## Purpose

Passing tests and independent review are necessary but not sufficient for promotion.

The integration Quality Gate also verifies repository contracts and architecture boundaries deterministically before a result can become promotable.

## OpenAPI compatibility

Existing files whose names contain `openapi` or `swagger` and end in JSON/YAML are compared between the original project and the final integrated workspace.

The gate blocks known breaking changes including:

- removal of an existing contract file;
- removal of an existing path;
- removal of an existing HTTP operation;
- change of an existing `operationId`;
- removal of an existing response status;
- change of an existing response contract;
- addition of a new required parameter;
- changing an optional parameter to required;
- changing an existing parameter schema;
- addition of a required request body;
- changing an existing request-body contract.

The checker is deliberately conservative where compatibility cannot be proven safely.

New endpoints, new response codes, and new optional parameters do not block promotion by themselves.

## Architecture policy

Projects may declare deterministic import boundaries in:

~~~text
.ai-engineering-os/architecture.yaml
~~~

Schema v1 example:

~~~yaml
version: 1
rules:
  - id: core-must-not-import-integrations
    paths:
      - "core/**"
    forbidden_imports:
      - "/integrations/"
~~~

Rules apply to Go, Java, TypeScript/TSX, and JavaScript/JSX source files.

Go imports are read with the Go parser rather than regex matching.

Java and JS/TS import declarations are extracted from their normal import syntax.

## Pre-existing architecture debt

Architecture policy is evaluated in both:

- the original project;
- the final integrated workspace.

Only **new** violations block promotion.

A pre-existing violation does not become a regression merely because the project already contained it.

This mirrors the baseline-aware Quality principle: old debt is visible, but it must not be confused with damage introduced by the current run.

## Promotion boundary

The order is:

~~~text
deterministic verification
  -> baseline-aware Quality
  -> independent review
  -> final original/workspace integrity
  -> final scope validation
  -> contract + architecture verification
  -> promotable=true
  -> explicit human promotion approval
~~~

Contract/architecture verification never promotes files and never bypasses the explicit approval boundary.
