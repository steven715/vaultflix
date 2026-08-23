# Architecture Decision Records

Short, **immutable** notes capturing *why* a non-obvious architectural choice was made — the alternatives rejected and the consequences accepted.

## Conventions

- One decision per file: `NNNN-kebab-title.md`.
- **Append-only.** Don't rewrite a past ADR to match new reality — supersede it with a new ADR and set the old one's status to `Superseded by ADR-NNNN`. This immutability is what keeps ADRs from rotting the way living docs (SPEC, plans) do.
- Format: Nygard-style — Status, Context, Decision, Alternatives rejected, Consequences.
- These were distilled from per-feature design/plan docs that were then removed; the full history remains in git. For new features, capture the durable decision here on merge — the working plan itself is ephemeral.

## Index

| ADR | Title | Status |
|-----|-------|--------|
| [0001](0001-local-disk-media-sources.md) | Keep videos on local disk, manage paths via a `media_sources` table | Accepted |
| [0002](0002-stream-auth-query-param-token.md) | Authenticate media streaming via a `?token=` query param | Accepted |
| [0003](0003-async-import-in-memory-no-queue.md) | Run async import as an in-memory goroutine, no job queue | Accepted |
| [0004](0004-soft-delete-users.md) | Soft-delete users via `disabled_at` | Accepted |
| [0005](0005-build-once-sha-image-compose-overrides.md) | Build the API once as a SHA-tagged image; layer envs with compose overrides | Accepted |
| [0006](0006-playback-hud-stall-classification.md) | Classify playback stalls as starved-vs-codec in the HUD | Accepted |
| [0007](0007-pwa-service-worker-cache-boundary.md) | PWA service worker never caches `/api` or `/minio`; prompt-to-update | Accepted |
| [0008](0008-xaccel-stream-offload.md) | Offload stream bytes to nginx via X-Accel-Redirect, env-gated | Accepted (supersedes the local-disk `http.ServeFile` prod path) |
| [0009](0009-jellyfin-scale-on-the-fly-streaming.md) | Position streaming as Jellyfin-scale on-the-fly, not YouTube-scale pre-processing | Accepted |

> Roadmap and current capabilities live elsewhere: see [ROADMAP.md](../../ROADMAP.md) (what's next) and [SPEC.md](../SPEC.md) (what's built).
