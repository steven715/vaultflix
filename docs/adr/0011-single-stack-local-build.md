# ADR-0011: One local-build stack; drop the GHCR artifact and the dev/prod split

- Status: Accepted
- Date: 2026-10-04
- Supersedes: [ADR-0005](0005-build-once-sha-image-compose-overrides.md)

## Context

ADR-0005 introduced a build-once SHA-tagged API image pushed to GHCR plus per-environment compose overrides (`docker-compose.prod.yml`, `docker-compose.test.yml`) on a dev base that ran `go mod tidy && go run` from a bind mount.

In practice, for a single-user, single-machine media server (ADR-0009):

- Nothing ever pulled the GHCR image — `task deploy` built locally — so "promote the same artifact" had no consumer.
- The stack actually left running day to day was the **dev** one: `go mod tidy` needed network on every start (an offline start failed the API outright), and X-Accel offload (ADR-0008) was off, so the configuration meant for daily use sat idle.
- `go run` gave no hot reload anyway — a code change still meant restarting the container.
- The prod override carried two documented merge traps (`!override` ordering with the media file, and `build.dockerfile` inheriting `Dockerfile.dev`).

## Decision

- `docker-compose.yml` is the **only** run stack: the API is the compiled-binary runtime stage of `Dockerfile`, behind nginx with X-Accel on and `restart: unless-stopped`. `task up` rebuilds and restarts it after code changes; BuildKit cache mounts keep the rebuild incremental.
- `docker-compose.test.yml` stays: it is isolation (ephemeral volumes, no host ports, fixture mount, own compose project), not a separate environment. Its API runs the same compiled image with X-Accel cleared; the ffmpeg Go tests run in a `go-test` service built from `Dockerfile`'s `dev` stage.
- `docker-compose.prod.yml`, `Dockerfile.dev`, the CI `build-push` job and `task build*` / `push:api` / `deploy` are removed.
- `npm run dev` proxies `/api` to nginx (`:3000`) rather than the API, since the API now always expects nginx in front.

## Alternatives rejected

- **Keep dev (`go run`) + release as two stacks** — two configs to drift, and the one used daily was the weaker one.
- **Fold `docker-compose.test.yml` into the base via profiles** — profiles cannot strip ports/volumes; the test stack would collide with the running one and could swap the fixture mount into the real API container.
- **Keep pushing to GHCR "for later"** — YAGNI; re-add a push step if a second host ever needs to pull.

## Consequences

- The integration suite now exercises the real runtime image, not `go run`.
- No Go toolchain in the running API container; debug via `docker compose run --build` on the `dev` target.
- A code change needs `task up` (an image rebuild) instead of a container restart.
- Deploying to another host means `git clone` + `task up` there; there is no registry artifact to pull.
- Still requires Compose v2.24+ for the `!override` tags in the test override.
