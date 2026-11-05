# Integration Promotion Candidate

## Purpose

A run-wide Quality PASS makes an integrated workspace eligible to become a promotion candidate, but it must not silently apply files to the original project.

This bridge connects the multi-agent integration pipeline to the existing promotion boundary without weakening approval requirements.

## Candidate creation

A candidate can be created only when:

- the Integration Quality Gate returned `Promotable=true`;
- the Quality result itself is `Passed=true`;
- the integrated workspace and pre-image exist;
- the final post-quality change set is non-empty.

The promotion request uses the **final** Quality-approved changes rather than the original pre-correction integration plan.

## Promotion boundary

The candidate can be passed to:

~~~text
promotion.DryRun(...)
~~~

for another deterministic integrity/scope check.

Actual mutation still requires:

~~~text
promotion.Apply(... Approved=true)
~~~

The bridge does not provide an auto-approval path.

## Safety sequence

~~~text
isolated task workspaces
  -> inspected/frozen outputs
  -> integration plan
  -> isolated integration workspace
  -> run-wide Quality Gate
  -> Promotable=true
  -> promotion candidate
  -> promotion dry-run
  -> explicit human approval
  -> promotion apply
~~~

Each boundary is independently testable and fail-closed.
