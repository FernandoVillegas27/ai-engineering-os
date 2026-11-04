# Isolated Task Workspaces

## Purpose

Write-capable multi-agent execution must not let several agents mutate the same shared staged tree.

The first isolation layer creates one sanitized writable workspace per mutating task while keeping the original project untouched.

This phase provides the workspace boundary only. It does **not** enable write-capable coordinated agents yet.

## Preparation

A task workspace is created from:

- original project root;
- task ID;
- agent ID;
- exact declared file/action scope.

Supported declared actions remain:

- `create`;
- `modify`;
- `delete`.

Preparation:

1. creates a fresh sibling temporary workspace outside the original project;
2. copies the same safe project view used by execution/staging;
3. excludes secrets, repository metadata, symlinks and unsupported files;
4. snapshots the copied tree;
5. creates an immutable `PreImage`;
6. validates each declared action against that pre-image.

A declared `create` must not already exist.

A declared `modify` or `delete` must exist.

## Inspection

After a future mutating worker finishes, the manager can inspect the isolated workspace.

Inspection:

- compares the workspace against its immutable pre-image;
- rejects sensitive, unsupported, symlink or special-file entries through existing staging policy;
- rejects any changed file not in the task's declared scope;
- rejects action mismatches.

Inspection never applies the changes to the original project.

## Multi-task conflicts

`ValidateDisjoint` checks exact declared file ownership across task workspaces.

Two task workspaces cannot both claim the same file.

This is stricter than optimistic LLM conflict resolution and provides a deterministic first integration rule.

The existing scheduler may already prevent overlapping high-level scopes; this workspace-level check protects the exact declared files once proposals become concrete.

## Cleanup safety

Cleanup only accepts paths inside the configured isolated workspace parent and explicitly refuses:

- the workspace parent itself;
- the original project;
- any path inside the original project;
- paths outside the isolated parent.

## Next phase

The next integration layer should consume multiple successfully inspected task workspaces and produce a deterministic **integration plan**.

That plan should:

- require a shared compatible pre-image;
- require disjoint exact file changes for the initial implementation;
- never write directly during planning;
- define deterministic application order;
- run run-wide quality after integration.

Only after that layer is tested should coordinated mutating workers be wired into `ai run execute`.
