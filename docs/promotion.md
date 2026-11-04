# Promotion Boundary

## Purpose

Promotion is the explicit boundary between an approved isolated staged workspace and the developer's original project.

P3a introduces **dry-run validation only**.

It does not copy, overwrite, delete, commit or push any file.

## Dry-run contract

A promotion request contains:

- the original project root;
- the isolated staged workspace;
- the immutable staging `PreImage`;
- the declared file/action scope;
- proof that the Quality Gate passed.

The dry run returns a promotion plan only when all invariants hold.

## Invariants

A plan is `Ready=true` only when:

1. a non-zero staging `PreImage` exists;
2. the Quality Gate passed;
3. original and staged workspaces are separate directories;
4. the declared scope contains only safe supported paths and actions;
5. a fresh sanitized view of the original still exactly matches the `PreImage`;
6. the staged workspace is safe to inspect;
7. every staged change is declared;
8. every staged action matches the declared action.

If the original project changed after staging, promotion is rejected.

If the staged workspace contains undeclared, sensitive, unsupported, symlink or special-file changes, promotion is rejected.

## Safety

The original project is read only.

The original comparison uses the same safe-view policy used by staging and baseline validation.

Temporary validation views are created outside the original project and removed after inspection.

Errors expose relative paths only where useful and scrub workspace locations from wrapped inspection errors.

## P3b

The next promotion phase may apply a validated plan.

P3b must re-check all invariants immediately before mutation and must define rollback/atomicity semantics before writing to the original tree.

No automatic commit or push is part of the promotion boundary.
