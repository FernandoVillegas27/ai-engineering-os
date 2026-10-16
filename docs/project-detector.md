# Project Detector

## Purpose

The Project Detector is the first executable core component of AI Engineering OS.

Its job is to inspect a project directory and return normalized technology detections backed by repository evidence.

It does not use an AI model.

## Principle

The detector implements the platform rule:

~~~text
Deterministic first, AI second.
~~~

If a repository contains machine-readable evidence, the platform should parse that evidence rather than ask a model to infer the stack.

## Current signals

The initial implementation supports:

- `package.json`
  - Angular
  - React
  - Next.js
  - TypeScript
  - Jest
  - Playwright
  - Node runtime
- `angular.json`
- `package-lock.json`
- `pnpm-lock.yaml`
- `yarn.lock`
- `pom.xml`
  - Java
  - Spring Boot
  - JUnit
  - Maven
- `go.mod`
  - Go
- `pyproject.toml`
  - Python
  - Django
  - pytest
- `Dockerfile`
- `docker-compose.yml`
- `docker-compose.yaml`

## Evidence

Every detected technology includes the file or files that caused the detection.

Example:

~~~json
{
  "name": "spring-boot",
  "version": "3",
  "evidence": ["pom.xml"]
}
~~~

This makes detection explainable and debuggable.

## What this step does not do

The detector does not yet:

- generate `.ai/project.yaml`;
- resolve skills;
- inspect architecture;
- invoke agents;
- use Engram;
- use AI for ambiguous detection;
- expose a CLI command.

Those responsibilities remain separate.

The future `ai inspect` command will compose this detector with the Project contract and Skill Resolver.

## Package

The implementation lives in:

`core/detector`

and currently uses only the Go standard library.
