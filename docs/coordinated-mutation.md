# Coordinated Mutation Opt-In

## Purpose

The main coordinated run path can now opt into write-capable multi-agent staging without weakening the original-project boundary.

The feature is intentionally explicit and fail-closed.

## Command

~~~text
ai run execute <project> \
  --task "..." \
  --runtime auto|codex|cursor \
  --multi-agent-write
~~~

Optional routing controls remain available:

~~~text
--model-policy auto|economy|balanced|deep
--model <id>
--max-concurrency <n>
~~~

## Pipeline

The opt-in path executes:

~~~text
classify + plan
  -> durable run
  -> Proposal Freeze
  -> exact ownership conflict check
  -> isolated StageRuntime per task
  -> filesystem re-inspection
  -> frozen ResultImage
  -> deterministic integration plan
  -> isolated integration workspace
  -> run-wide baseline-aware Quality Gate
  -> promotion candidate
  -> promotion dry-run
  -> approval.requested
~~~

## Original project boundary

The command does **not** call `promotion.Apply`.

Therefore it does not:

- copy integrated files into the original project;
- commit;
- push;
- auto-approve promotion.

The only retained mutation output is the final isolated integration workspace when all gates pass.

Temporary per-agent stage workspaces are removed after integration.

## Explicit opt-in

The write-capable path runs only when `--multi-agent-write` is present.

It cannot be combined with `--dry-run`.

Without the flag, the existing integrated read-only path retains its current runtime restrictions.

## Runtime routing

The mutation path reuses the same provider/model routing helpers already used by `ai propose` and `ai stage`:

- proposal generation;
- stage execution;
- independent review;
- correction.

No model is hard-coded to an agent role.

## Durable state and Control Plane

The run is persisted before mutation starts.

The mutation path records Control Plane events for:

- run start;
- task creation/assignment;
- frozen proposal ownership;
- task stage completion;
- run-wide verification;
- review approval;
- promotion approval request.

If a phase fails, the durable run transitions to `failed`.

## Successful terminal state

A successful opt-in run reports:

- run ID;
- integrated workspace path;
- Quality outcome;
- promotion candidate READY;
- approval required YES.

The run can then be inspected with:

~~~text
ai run control <run-id>
ai run events <run-id>
~~~

The integration workspace remains outside the original project until a separate explicit promotion action is introduced.
