# ADR-0012: Test repository SQL against the integration stack's real Postgres

- Status: Accepted
- Date: 2026-10-10

## Context

Every read of a Video built its own column list and scan, and they drifted: some read Code and some didn't, and none read back the Metadata Enrichment writes (Maker, Label, Series, release date, runtime, Cover key, enriched_at). Fix cb2d039 patched one of those queries after a shell integration script happened to notice.

That class of bug — "the SQL doesn't select a column" — cannot be caught by the service tests, because they replace the repository with hand-written mocks that return whatever the test wrote. The repository package itself had no Go tests that touched a database. `watch_session_repo_test.go` had explicitly declined to invent a Go DB harness "without precedent", leaving SQL coverage to the HTTP-level `scripts/test_*.sh` suites. Those suites only see what an endpoint happens to expose.

Since then, `task test-integration` gained a `go-test` service. It runs Go tests on the Dockerfile `dev` stage, on the integration stack's compose network, after migrations — currently the ffmpeg-dependent streaming tests.

## Decision

- Repository tests that need SQL run against the integration stack's migrated Postgres, inside the existing `go-test` service.
- `openTestPool` reads `VAULTFLIX_TEST_DATABASE_URL`. When it is unset the test is skipped, so the native `task verify` gate stays DB-free. `VAULTFLIX_REQUIRE_DB=1` (set in `go-test`) turns that skip into a failure, the same pattern as `VAULTFLIX_REQUIRE_FFMPEG`.
- Tests seed their own rows with unique names and delete them in `t.Cleanup`, because the shell suites run afterwards against the same database.
- The service-layer rule stays: services test against mocked repositories. Only the repository's own SQL is tested for real.

## Alternatives rejected

- **Assert through the HTTP suites only** (e.g. `GET /videos/:id` returns `maker` after accept): covers one endpoint per assertion, and can't state the invariant "every reader returns a complete Video".
- **A separate throwaway Postgres (testcontainers / embedded)**: a new dependency and a second way to start Postgres, when the integration stack already provides a migrated one.
- **Unit-test the SQL strings** (e.g. assert each query contains `videoColumns`): tautological, passes even when the scan order is wrong.

## Consequences

- Repository tests don't run in `task verify`. They run in `task test-integration` and CI.
- The `go-test` container now needs the Go modules for pgx. On a network that can't reach `proxy.golang.org`, the step fails at module download — it doesn't skip.
- A model field added without reading it back fails `TestVideoRepository_GetByID_ReadsEveryColumn`, which checks every `model.Video` field by reflection.
