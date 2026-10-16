# Project Contract

## What a project is

A project is the normalized technical context that AI Engineering OS uses to understand a repository before selecting agents, skills and tools.

The persisted file uses a top-level `project` key:

~~~yaml
project:
  id: example
  name: example
~~~

A project definition should answer questions such as:

- What stack is this repository using?
- Which framework and runtime versions were detected?
- Which build and testing tools are present?
- What architecture is known?
- Which skills are approved and recommended?
- Which matching skills are still candidates?
- Which detected technologies have no matching skill?
- Which memory namespace belongs to this project?

## Generated, not manually maintained

Most of this file is generated or refreshed by `ai inspect`.

The user should not need to manually describe a project that can be inspected deterministically from repository evidence.

For example:

~~~text
package.json
angular.json
pom.xml
build.gradle
pyproject.toml
go.mod
Cargo.toml
Dockerfile
docker-compose.yml
*.csproj
~~~

These files are stronger evidence than asking a model to guess the stack.

## Repository

The repository section stores stable repository context such as Git usage, remote and default branch.

Example:

~~~yaml
repository:
  type: git
  remote: git@github.com:company/project.git
  default_branch: main
~~~

## Detected technologies

Detected technology is grouped by engineering concern:

- `frontend`
- `backend`
- `database`
- `testing`
- `build`
- `runtime`
- `infrastructure`

Each technology can include a version and evidence.

Example:

~~~yaml
backend:
  - name: java
    version: "21"
    evidence:
      - pom.xml
  - name: spring-boot
    version: "3"
    evidence:
      - pom.xml
~~~

The `evidence` field exists so detection remains explainable.

## Architecture

Architecture should not be invented when the repository does not provide enough evidence.

The default remains:

~~~yaml
architecture:
  style: unknown
~~~

Later, an architect agent may propose or confirm an architecture, but deterministic detection should not fabricate one.

## Skill states in a project

Resolved skills are preserved in three groups.

### Recommended

`recommended` contains matching skills whose registry status is `enabled`.

~~~yaml
skills:
  recommended:
    - company-spring-guidelines
~~~

These are eligible for automatic use by later runtime stages.

### Candidates

`candidates` contains relevant skills that matched the project but are not yet approved.

~~~yaml
skills:
  candidates:
    - java-21
    - spring-boot-3
~~~

This preserves discovery without silently turning third-party guidance into trusted company knowledge.

### Missing

`missing` contains detected technologies for which the current registry did not produce a compatible skill match.

~~~yaml
skills:
  missing:
    - oracle
~~~

This lets the system later recommend importing, creating or reviewing a skill rather than silently proceeding without specialized knowledge.

## Memory namespace

Every project can have its own memory namespace.

Example:

~~~yaml
memory:
  provider: engram
  namespace: supervielle-deuda-tc
~~~

This prevents unrelated projects from sharing project-specific engineering context.

The memory provider itself remains configurable so the Project contract is not permanently coupled to Engram.

## Deterministic first

Project detection is one of the clearest examples of the platform principle:

~~~text
Deterministic first, AI second.
~~~

If `pom.xml` contains Java 21 and Spring Boot 3 metadata, the system should parse that evidence.

It should not ask a language model:

> Which backend framework do you think this repository uses?

AI can still help when evidence is ambiguous, incomplete or architectural rather than factual.

## Relationship with the rest of the system

~~~text
Repository
   ↓
Project Detector
   ↓
Project
   ↓
Skill Resolver
   ↓
recommended / candidates / missing
   ↓
Agents + Skills + Tools
   ↓
Execution
~~~

The Project contract is therefore the bridge between a real codebase and the rest of AI Engineering OS.

## Validation

The machine-readable contract is:

`schemas/project.schema.json`

A sample project is available at:

`examples/project.yaml`

## Design rules

1. Project context should be derived from repository evidence whenever possible.
2. Detected technologies should include evidence.
3. Versions should be captured when deterministically available.
4. Unknown architecture should remain unknown rather than guessed.
5. Enabled skills and candidate skills must remain distinguishable.
6. Missing technology coverage should be explicit.
7. Memory should be isolated by project namespace.
8. Project definitions must remain independent from IDEs and AI model vendors.
