# ADR-0007: PWA service worker never caches `/api` or `/minio`; prompt-to-update

- Status: Accepted
- Date: 2026-06-26

## Context

Streaming is inherently online. A service worker that cached `/stream`, MinIO assets, or API routes would serve stale data or break playback, and an auto-reload mid-playback is unacceptable.

## Decision

- The SW precaches only the static app shell (`js/css/html/svg/woff2`) and forces **NetworkOnly** for any `/api` or `/minio` request, with `navigateFallbackDenylist` for both prefixes.
- `registerType: 'prompt'` — a new version never auto-reloads; the user is prompted via the existing Toast.
- Iron rule: the SW must not touch `/api` (especially `/stream`) or `/minio`.

## Alternatives rejected

- **`registerType: 'autoUpdate'`** — can reload during playback.
- **Default Workbox caching of all routes** — would intercept streaming/API.

## Consequences

- Adds the `vite-plugin-pwa` build dependency (a third-party dep, which per CLAUDE.md required prior approval).
- Offline value is limited to the app shell.
- iOS standalone has a separate storage jar → a token re-login may be needed on first launch.
- Mis-caching risk ("changed but seeing old version") is mitigated by `prompt` + nginx no-cache on `sw.js`/`index.html` + immutable hashed assets.
