# Cursor Projection Validation

## Purpose

`ai gentle validate cursor` validates the effective Cursor projection after Gentle-AI sync.

The goal is not to prove that every AI behavior will be correct. It verifies the deterministic configuration boundary that AI Engineering OS depends on.

## Checks

The initial validator checks:

- Gentle-AI's Cursor rule exists;
- the rule is `alwaysApply: true`;
- current ODD guidance is present;
- active legacy SDD routing is not present in the rule;
- Engram MCP is registered and points to an existing executable;
- Engram uses the expected agent MCP arguments;
- Context7 MCP is registered;
- Judgment Day review agents are present;
- legacy `sdd-*` skill or agent files are reported if they remain.

## Legacy SDD policy

Gentle-AI v4 retired SDD in favor of ODD.

A v4 sync removes legacy SDD assets when their ownership can be proven. Edited or otherwise unrecognized legacy files may be preserved instead of deleted.

AI Engineering OS treats this distinction as:

~~~text
active SDD routing in gentle-ai.mdc
→ ERROR

orphaned/preserved sdd-* files
→ WARN

no SDD remnants
→ OK
~~~

This avoids deleting user-edited files automatically while still making legacy state visible.

## MCP policy

The validator does not require one exact Context7 transport or package version.

Context7 may be local or remote and Gentle-AI intentionally preserves customized entries.

For Engram, the configured executable must exist because a stale executable path breaks persistent memory at runtime.

## Usage

~~~powershell
go run ./cmd/ai gentle validate cursor
~~~

Expected healthy result:

~~~text
Cursor Projection

[OK] Gentle Rule      Regla alwaysApply activa con routing ODD.
[OK] Engram MCP       Engram MCP apunta a un ejecutable válido.
[OK] Context7 MCP     Context7 MCP está registrado en Cursor.
[OK] Review Agents    Judgment Day agents presentes.
[OK] Legacy SDD       No se detectaron assets SDD legacy en Cursor.

Resultado: listo.
~~~

## Boundary

This command is inspection-only.

It does not:

- delete legacy assets;
- run Gentle-AI sync;
- rewrite Cursor settings;
- modify MCP configuration;
- enable or disable RDD;
- change models.

Any cleanup remains explicit and reviewable.
