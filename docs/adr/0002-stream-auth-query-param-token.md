# ADR-0002: Authenticate media streaming via a `?token=` query param

- Status: Accepted
- Date: 2026-04-06

## Context

The streaming endpoint `/api/videos/:id/stream` is consumed by an HTML `<video src>` element, which cannot attach an `Authorization` header. The same limitation applies to WebSocket upgrades, SSE, and plain file downloads.

## Decision

The JWT auth middleware reads the token from `Authorization: Bearer` **first**, then falls back to a `?token=` query parameter (header wins). A short-lived, scope-limited stream token is minted for the `<video>` URL (`STREAM_TOKEN_EXPIRY_MINUTES`).

## Alternatives rejected

- **Cookie-based auth for streaming** / **per-stream signed URLs** — noted in code as what production-grade systems "should" use, but over-engineering for a self-hosted single-user deployment.

## Consequences

- The token appears in nginx/server access logs and browser history. Explicitly accepted for self-hosted use; the trade-off is documented in `auth.go` so future readers don't "fix" it blindly.
- Widens the auth surface (tokens flow through URLs); mitigated by the short stream-token lifetime + transparent client refresh.
- Survives the X-Accel refactor unchanged — auth still runs in Go middleware before the redirect (see [ADR-0008](0008-xaccel-stream-offload.md)).
