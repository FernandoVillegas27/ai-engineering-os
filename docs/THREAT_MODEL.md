# Threat model — AI Engineering OS

Status: initial threat inventory for release gate R1. Not a completed security audit.
Updated: 2026-11-06.

## Assets and boundaries

1. **Original repository**: source code and configuration must remain unchanged until separately approved promotion.
2. **Isolated workspaces**: agent-written content and tests are untrusted; isolation and file-scope validation are mandatory.
3. **Run event store, snapshots, archives and checkpoints**: durable evidence must resist tampering, truncation and replay confusion.
4. **Artifact/message bus and memory**: model-generated and retrieved material is untrusted data, never an instruction or permission.
5. **Runtime and tool adapters**: spawned processes, MCP tools, connectors and local utilities may execute commands or make network calls.
6. **Control Plane / CLI**: authorized human actions must be distinguished from model requests.
7. **Secrets, source files and diagnostic output**: logs and tool errors must not become an exfiltration channel.

## Attacker-controlled inputs

- Project repository content, including malicious test scripts, filenames, dependencies and symlinks.
- LLM responses, copied code, suggested tool arguments and collaboration messages.
- MCP/connector responses, network content and stored memory.
- Environment variables, runtime command stdout/stderr, malformed registry or policy files.
- Persisted event/archive files altered by another local process or interrupted writes.

## Critical scenarios and required defenses

| Threat | Boundary | Expected safe result | Validation to add |
| --- | --- | --- | --- |
| `../` or absolute path write | workspace -> original root | deny unauthorized escape | path traversal test |
| Windows junction or Unix symlink | task scope -> host filesystem | reject escape even under aliasing | OS-specific links |
| Changed original file after validation | verification -> promotion | refuse stale preimage | concurrent modification |
| Prompt injection in README / retrieved docs | untrusted evidence -> instructions | preserve role and permissions | hostile instruction fixture |
| Tool response requests privileged operation | external data -> tool authorization | deny unless independently authorized | forged tool output |
| Sensitive environment leaked in failed command | subprocess -> logs / events | bounded redacted diagnostics | injected credential fixture |
| Archive bytes/hash tampered | persisted history -> replay | fail closed with explicit error | tamper matrix |
| Snapshot covers absent prefix | snapshot -> compaction | reject destructive operation | interrupted history fixture |
| Cancel during child process | coordinator -> runtime | bounded stop and no orphan process | process-tree cancellation |
| Repeated retryable provider failures | tool health -> orchestrator | bounded retries and terminal blocker | quota/timeout fixture |
| Stored stale memory conflicts with current run | memory -> team decision | provenance-aware untrusted context | conflicting decision fixture |
| Agent self-approval | mutation -> promotion | independent review and human approval | approval bypass test |

## Security invariants

- No runtime is granted new permission because a tool is healthy or a model asks for it.
- No automatic promotion to the original repository.
- No pre-existing test failure, partial parse or ambiguous infrastructure failure is converted into a pass.
- No unverified event archive authorizes deletion of live history.
- No source-file or recalled-memory instructions override deterministic policy.
- No external tool can bypass a scoped task's permission envelope.
- No retry loop is unbounded.

## Evidence and operational status

The repository already contains protective mechanisms such as workspace integrity checks, explicit promotion, durable run event validation, archive checks and normalized external health (see corresponding merged PRs). This inventory does **not** assert that all attack scenarios above are covered by tests.

Before releasing unattended autonomy:
1. assign an owner and severity to each scenario;
2. implement executable adversarial tests on all supported OSes;
3. capture failure-type and invariant evidence in CI;
4. resolve every critical/high bypass or explicitly remove the affected capability from release scope;
5. record residual risks and approve the release gate.

No production-readiness approval is given by this document.
