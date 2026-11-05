# Run-Wide Integration Quality Gate

## Purpose

An isolated integration workspace is not a promotion candidate merely because every specialist task completed.

The combined result must pass a run-wide deterministic verification and independent review after integration.

## Inputs

The integration Quality Gate receives:

- original project root;
- deterministic integration plan;
- isolated integrated workspace;
- high-level user task;
- verifier;
- independent reviewer;
- optional bounded corrector.

## Baseline

The gate constructs the same integrity-safe lazy baseline provider used by the normal Quality Loop.

It binds:

- original project;
- integration workspace pre-image;
- exact planned file paths;
- the same verifier used on the integrated workspace.

This preserves the distinction between pre-existing failures and new regressions after multi-agent integration.

## Before quality

The gate verifies that:

- integration workspace pre-image matches the plan;
- the workspace's initial combined change set exactly matches the plan;
- verifier/reviewer/task are configured.

## Quality Loop

The existing baseline-aware `quality.Loop` is executed on the integrated workspace.

The same rules continue to apply:

- compile/build blockers block;
- partial structured verification blocks;
- integrity failures block;
- pre-existing-only failures never become a false PASS;
- repeated failure fingerprints stop correction;
- independent review is required.

## Final scope validation

Correctors may only operate inside the planned file set.

After the Quality Loop, the gate re-inspects the workspace and rejects:

- any new unauthorized path;
- action changes outside the plan;
- a fully reverted/no-change result.

A corrector may change the contents of an already-authorized file when repairing a regression.

## Final original integrity check

A clean staged verification may not need a lazy baseline during the Quality Loop.

Therefore, before returning `Promotable=true`, the gate explicitly forces the baseline provider to perform a final original/staged integrity check.

The integration workspace is inspected again after that verification window.

## Result

The gate returns:

- full structured Quality result;
- final change set;
- `Promotable` boolean.

`Promotable=true` requires:

1. Quality Loop PASS;
2. final authorized scope;
3. original project still matching the pre-image;
4. final integration workspace still valid.

This flag does not itself apply or promote files.

The existing approval-gated promotion boundary remains a separate explicit step.
