# ADR-0005: Build the API once as a SHA-tagged image; layer envs with compose overrides

- Status: Accepted
- Date: 2026-06-21

## Context

The API previously ran `golang:1.24-alpine` + bind mount + `go mod tidy && go run` at container start — not an immutable, env-agnostic artifact. Naively duplicating compose files per environment causes drift, and Compose's default volume merge **appends** rather than replaces (the "volume merge-append trap").

## Decision

- Build the Go API **once** into a SHA-tagged immutable image pushed to GHCR (`ghcr.io/steven715/vaultflix-api`); promote the *same* artifact to every environment. Version is the only build-time bake (`-ldflags -X main.version`).
- `docker-compose.prod.yml` / `docker-compose.test.yml` layer **minimal** overrides on the base file using Compose `!reset` / `!override` (requires Compose **v2.24+**) instead of copying infra definitions.

## Alternatives rejected

- **Rebuild per environment** — violates build-once.
- **Separate full compose files per env** — drift.
- **golangci-lint / heavier tooling up front** — deferred (YAGNI; start with `go vet` + `gofmt`, measure first).
- **Push the web image to GHCR** — dropped; the web SPA stays a local nginx-image build.

## Consequences

- Hard floor: every host needs Compose v2.24+.
- `!reset` / `!override` are less obvious than plain YAML and need familiarity.
- Env-specific behavior must come from injected `.env` / runtime params, never the image — enforcing the config-three-ways split (build-time ldflags / deploy-time `.env` / runtime API params).
- The artifact + override mechanics are summarized in CLAUDE.md「CI/CD 與單一入口」; this ADR records the rationale.
