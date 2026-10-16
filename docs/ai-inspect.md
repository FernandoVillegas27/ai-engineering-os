# ai inspect

`ai inspect` is the first user-facing command of AI Engineering OS.

It composes the deterministic Project Detector and Skill Resolver into one project inspection flow.

## What it does

~~~text
Project directory
      ↓
Project Detector
      ↓
Detected technologies + evidence
      ↓
Skill Registry
      ↓
Skill Resolver
      ↓
Project context
~~~

The resulting context includes:

- project identity;
- Git repository metadata when available;
- detected technologies and evidence;
- architecture state;
- enabled skill recommendations;
- candidate skills;
- unmatched technologies;
- project memory namespace.

## Usage

From the AI Engineering OS repository:

~~~bash
go run ./cmd/ai inspect /path/to/project
~~~

This prints the generated YAML without modifying the target project.

To persist the result:

~~~bash
go run ./cmd/ai inspect /path/to/project --write
~~~

This creates:

~~~text
.ai/project.yaml
~~~

inside the inspected project.

## Skill registry location

The command resolves the skill registry in this order:

1. explicit `--registry`;
2. `AI_ENGINEERING_OS_HOME/registry/skills.yaml`;
3. `registry/skills.yaml` in the current working directory.

A future installation layer will remove the need to manage this path manually.

## Candidate skills

Candidate skills are intentionally preserved separately from enabled skills.

Example:

~~~yaml
skills:
  recommended: []
  candidates:
    - java-21
    - spring-boot-3
  missing:
    - maven
~~~

A candidate match means:

> The skill is relevant, but it has not yet been approved for automatic use.

## Safety

Without `--write`, `ai inspect` is read-only.

This makes inspection safe to run before deciding whether the project should persist AI Engineering OS metadata.

## Language

Internal contracts and machine-readable values remain in English.

User-facing CLI messages are in Spanish for the team.
