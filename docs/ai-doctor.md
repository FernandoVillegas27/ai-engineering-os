# ai doctor

`ai doctor` provides one deterministic health report for the local AI Engineering OS environment.

It does not call an AI model.

## Purpose

Individual commands already exist for specific providers:

~~~text
ai memory status
ai docs status
~~~

As the platform grows, checking each dependency manually becomes inefficient.

`ai doctor` consolidates the environment into a single report.

## Initial checks

The first version checks:

- workspace accessibility;
- Git availability and repository detection;
- Skill Registry loading;
- persisted AI Engineering OS project context;
- Engram project scoping;
- Engram provider health;
- Gentle-AI installation/health;
- Context7 launcher readiness.

## Example

~~~text
AI Engineering OS Doctor

[OK]   Workspace       Workspace accesible.
[OK]   Git             Repositorio Git detectado.
[OK]   Skill Registry  Registry válido (8 skills).
[WARN] Project Context .ai/project.yaml todavía no existe.
[OK]   Engram Project  Scope fijado en ai-engineering-os.
[OK]   Engram          engram 3.4.0
[WARN] Gentle-AI        Gentle-AI todavía no está instalado.
[OK]   Context7        npx 10.2.4 · @upstash/context7-mcp@4.4.2

Resultado: listo con advertencias
~~~

## Severity

The report uses three levels:

- `OK`: the check is ready;
- `WARN`: the platform can continue, but configuration is incomplete or deserves attention;
- `ERROR`: a required part of the current environment is unavailable or invalid.

Missing `.ai/project.yaml` is intentionally a warning because inspection can still run without persisting project context.

## Project path

The command accepts an optional project path:

~~~bash
ai doctor
ai doctor /path/to/project
~~~

When omitted, the current directory is used.

## Registry resolution

The Skill Registry uses the same resolution rules as `ai inspect`:

1. explicit `--registry`;
2. `AI_ENGINEERING_OS_HOME/registry/skills.yaml`;
3. local `registry/skills.yaml`.

## Design principle

Health checks should be deterministic and as side-effect free as possible.

`ai doctor` should inspect state, not repair it automatically.

Future fix commands may be added separately when explicit mutation is appropriate.

## Future checks

Possible later additions include:

- CodeGraph;
- Playwright;
- agent runtime projection;
- tool permission validation;
- schema validation;
- model-provider readiness;
- GitHub authentication;
- project-specific quality gates.
