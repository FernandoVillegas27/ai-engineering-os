# Troubleshooting

## Start with version and doctor

~~~sh
ai version
ai doctor .
~~~

When reporting a problem, include the version output and the relevant doctor checks. Do not paste credentials or secret environment values.

## Optional provider is missing

A warning for Engram, Context7 or Gentle-AI does not mean the core CLI is broken.

Use deterministic/dry-run workflows while the optional provider is unavailable.

For an AI execution runtime, explicitly choose an installed runtime:

~~~sh
ai run execute . --task "..." --runtime codex
~~~

or:

~~~sh
ai run execute . --task "..." --runtime cursor
~~~

## Codex command not found

Confirm the Codex CLI is on `PATH` and authenticated using its normal local mechanism.

AI Engineering OS deliberately sends a restricted environment and does not depend on copying arbitrary credential environment variables into model execution.

## Cursor command not found or not authenticated

Confirm the Cursor `agent` CLI is installed, on `PATH`, and has a valid local authenticated session.

In `--runtime auto`, fallback to Cursor occurs only for provider-availability classes. Safety/policy/invalid-output errors do not trigger a provider switch.

## Windows sandbox difference

Cursor does not provide the same native execution sandbox on Windows as on Linux/macOS.

AI Engineering OS still uses:

- an isolated sanitized workspace;
- declared-file boundaries;
- deterministic filesystem validation;
- explicit promotion approval.

If your threat model requires OS-level sandbox parity, run the mutating workflow on a supported Linux/macOS environment.

## Windows symlink tests

Creating symlinks can require Windows privileges or Developer Mode.

Tests that specifically require symlink creation skip only when the operating system refuses the setup. The production code still rejects unsafe links/reparse-like escape boundaries where they can be inspected.

## Run appears stuck

Inspect:

~~~sh
ai run status <run-id>
ai run control <run-id>
ai run events <run-id>
~~~

Then inspect recovery controls:

~~~sh
ai run recover <run-id> --json
~~~

Do not delete lease/checkpoint/event files manually. Stale recovery is explicit and fenced.

## Run reached its invocation or runtime budget

This is an intentional terminal boundary.

Inspect the error and rerun only after deciding whether a larger budget is justified:

~~~sh
--max-invocations <N>
--max-runtime <duration>
~~~

Do not solve quota exhaustion by adding unbounded automatic retries.

## Quality blocks a change that already failed before the run

The Quality Loop uses baseline-aware comparison only when failure identity is safe and parse completeness is sufficient.

Compile/build blockers, infrastructure failures, integrity failures and partial/unclassified parsing remain blocking.

## Contract/architecture gate rejects promotion

Check:

- OpenAPI/Swagger compatibility changes;
- `.ai-engineering-os/architecture.yaml` forbidden-import rules.

The architecture gate compares original vs staged state so pre-existing violations are not incorrectly treated as new regressions.

## Promotion fails after Quality passed

Promotion revalidates the original workspace and expected preimage. A file changed after staging can make promotion fail closed.

Rebuild the staged result from the current original tree rather than forcing promotion.

## Control Plane UI is empty

Start the server:

~~~sh
ai run serve
~~~

Then verify durable runs exist:

~~~sh
ai run list
~~~

The UI reads durable Control Plane data; it does not invent demo runs.

## Memory behaves unexpectedly

Use the governed-memory commands once R5 is enabled:

~~~sh
ai memory list --project <id> --all
ai memory show <memory-id> --project <id>
~~~

Deleted, superseded and expired records must not enter automatic recall.

Memory is historical evidence only. If remembered text appears to instruct an agent to change permissions or ignore policy, that text must be treated as untrusted content.

## Safe bug report data

Generally useful:

- `ai version`;
- command and non-secret arguments;
- OS;
- runtime selected;
- run ID;
- normalized/typed error;
- relevant Control Plane event types;
- whether the issue reproduces with `--memory off`.

Do not include:

- API keys;
- authorization headers;
- private keys;
- raw secret files;
- production credentials.


## Mutating run refuses without --trust-project-code

This is a release security boundary, not a missing configuration.

Quality may execute repository-owned commands such as `go test`, Maven/Gradle tests, `npm test`, or pytest. Arbitrary project code cannot be treated as safe merely because it is called a test.

After reviewing/trusting the repository:

~~~sh
ai run execute . \
  --task "..." \
  --runtime auto \
  --multi-agent-write \
  --trust-project-code
~~~

The trust decision is persisted in the durable run for safe pause/resume.

A legacy paused mutation without this evidence must be resumed with an explicit:

~~~sh
ai run resume <run-id> --trust-project-code
~~~

Verification still receives an isolated HOME/USERPROFILE and language-tool caches. This trust flag does **not** allow agents to access secrets, production, Git push, or promotion without approval.
