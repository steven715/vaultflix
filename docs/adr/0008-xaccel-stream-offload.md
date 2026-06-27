# ADR-0008: Offload stream bytes to nginx via X-Accel-Redirect, env-gated

- Status: Accepted — supersedes the local-disk `http.ServeFile` **prod** byte path (the dev/fallback path remains)
- Date: 2026-06-26

## Context

Under the prior design ([ADR-0001](0001-local-disk-media-sources.md)) every GB streamed through the Go process via `http.ServeFile`, pinning Go on byte-shuffling. But `npm run dev` proxies straight to Go with **no nginx**, where an X-Accel header would break the browser; mount paths contain CJK / spaces; and only the API container — not nginx — had the video volume mounted.

## Decision

- nginx serves the bytes from an `internal` location reading directly off disk with native Range. Go only validates the stream token + path safety, then returns an `X-Accel-Redirect` header (empty body).
- A `VIDEO_XACCEL_PREFIX` env var switches behavior: empty → Go `http.ServeFile` (dev / vite / unit tests); `/internal-video/` → X-Accel offload (prod behind nginx). This is justified as an **infrastructure/topology** fact ("is there an accel-capable proxy behind me?"), an explicit exception to CLAUDE.md's "business params go in API requests" rule.
- nginx must mount the **same** `:ro` disks as the API; Content-Type comes from nginx's `types` block keyed on file extension.

## Alternatives rejected

- **Keep `http.ServeFile` everywhere** — Go stays pinned by byte serving.
- **Always emit X-Accel unconditionally** — dev/test paths don't go through nginx and would break.

## Consequences

- Second mount consumer: adding a disk now touches API + nginx (mitigated with a YAML anchor / inheritance).
- DB `MimeType` no longer applies to disk files (only 5 import extensions; keep the `types` block in sync).
- Two code paths (ServeFile vs accel) both need testing.
- **Highest risk:** forgetting `internal;` on the nginx location is a security hole — anyone could `GET /internal-video/...` bypassing the stream token. Acceptance must test that external direct hits return 404.
- Does **not** speed up streaming (upstream tunnel bandwidth is the real bottleneck); it's a "Go isn't pinned" optimization only.
