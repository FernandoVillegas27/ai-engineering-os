# Persisted Promotion Candidates

## Purpose

A successful coordinated mutating run can now persist its promotion candidate by run ID.

This decouples promotion review/approval from the process that created the integrated workspace.

## Stored record

The candidate record contains:

- run ID;
- original project root;
- integrated workspace path;
- full immutable pre-image snapshot;
- pre-image digest;
- final Quality-approved declared changes;
- Quality-passed flag;
- temp parent;
- creation timestamp.

The persisted pre-image hashes are internal safety evidence.

## Integrity

Loading a record validates:

- schema version;
- safe run ID;
- required paths;
- Quality passed;
- non-empty declared changes;
- valid pre-image hashes;
- reconstructed pre-image digest matches the persisted digest;
- supported create/modify/delete actions;
- timestamp.

Candidate files are immutable per run ID.

Symlink candidate entries are rejected.

## CLI inspection

~~~text
ai run promotion <run-id>
~~~

Optional:

~~~text
--promotion-dir <path>
--json
~~~

The command reconstructs the original promotion request and runs `promotion.DryRun` again.

If the original project or integrated workspace changed since candidate creation, the candidate no longer reports READY.

The CLI does not expose the individual pre-image hashes.

## Storage resolution

Promotion candidate storage resolves in this order:

1. explicit `--promotion-dir`;
2. `AI_ENGINEERING_OS_PROMOTION_DIR`;
3. OS user config directory under `ai-engineering-os/promotion-candidates`.

## Approval boundary

Persisting or inspecting a candidate never calls `promotion.Apply`.

A later explicit approval command can load this record and call the existing approval-gated apply boundary.


## Explicit apply

A persisted candidate can be applied only through an explicit approval command:

~~~text
ai run promote <run-id> --approve
~~~

Optional storage overrides:

~~~text
--promotion-dir <path>
--state-dir <path>
--events-dir <path>
--json
~~~

The command performs a fresh `promotion.DryRun` before calling `promotion.Apply`.

Therefore a candidate that became stale because the original project or integrated workspace changed is rejected before application.

The existing promotion layer still provides:

- explicit approval enforcement;
- per-file state revalidation;
- rollback storage;
- rollback on apply failure;
- final original-vs-staged verification.

After successful application:

- the durable run records a granted `promotion` approval;
- the Control Plane records `approval.granted`;
- the persisted candidate remains as immutable audit evidence;
- no Git commit or push occurs automatically.

A successful coordinated mutating run now emits `run.completed` once the promotion candidate is ready. Promotion approval may be audited after that terminal event, but task/verification execution cannot resume implicitly.
