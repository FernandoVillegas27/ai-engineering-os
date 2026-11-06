# Embedded Control Plane Web Client

## Purpose

The Control Plane web client is a human-facing, read-only view over the existing durable Control Plane.

It is **not** a second coordinator and it does not persist its own run state.

The authoritative sources remain:

- Control Plane event history;
- snapshot/archive-backed projection;
- persistent messages and artifacts;
- promotion candidates;
- deterministic run-control capability derivation.

## Start

~~~text
ai run serve
~~~

Default local URLs:

~~~text
Dashboard: http://127.0.0.1:8765/ui/
API:       http://127.0.0.1:8765
~~~

The server remains restricted to explicit loopback addresses.

## Dashboard surfaces

The embedded client shows:

- durable run list and progress;
- task execution graph projection;
- agent state;
- event timeline;
- persistent inter-agent messages;
- immutable artifacts;
- promotion candidate readiness;
- deterministic pause/resume/cancel/retry/recovery capability state.

The UI is served from files embedded in the Go binary. It requires no Node runtime, frontend build, CDN, or external asset host.

## Live updates

The selected run connects to the existing SSE endpoint:

~~~text
GET /runs/<run-id>/stream?after=<sequence>
~~~

Events trigger refreshes from the normal read APIs.

A slower polling loop remains as a fallback for reconnects, browser suspension, or event types unknown to an older client.

The browser does not project event state independently.

## Read-only boundary

The web client does not issue:

- POST;
- PUT;
- PATCH;
- DELETE.

The Controls tab displays deterministic capabilities only.

Pause, resume, cancel, retry, recovery, and promotion continue to require their existing explicit CLI/control boundaries.

## Browser security

The embedded server applies:

- `X-Content-Type-Options: nosniff`;
- `X-Frame-Options: DENY`;
- `Referrer-Policy: no-referrer`;
- `Cache-Control: no-store`;
- a same-origin Content Security Policy;
- a restrictive Permissions Policy.

Scripts can load only from the same embedded origin.

Network connections are restricted to the same origin, which is where the Control Plane API/SSE live.

## Architecture boundary

The browser is intentionally replaceable.

A future VS Code or IntelliJ client can consume the same read-only Control Plane API without changing orchestration, event history, snapshots, recovery, Quality, memory, or promotion semantics.
