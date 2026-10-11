# ADR-0003: Run async import as an in-memory goroutine, no job queue

- Status: Accepted
- Date: 2026-04-06

## Context

`POST /api/videos/import` was synchronous; large libraries (≈18 GB / 4m40s) caused HTTP timeouts and gave the user no feedback. Single-user deployment.

## Decision

- Return `202 Accepted` + a job ID immediately; run the import in a background goroutine using `context.Background()`.
- Track job state in an in-memory `sync.Map`; enforce single-job-at-a-time with a `sync.Mutex` (`TryLock` → `ErrConflict` / 409).
- Stream per-file progress over WebSocket (`import_progress` / `import_complete` / `import_error`); the frontend re-queries `GET /import-jobs/active` on mount to recover.

## Alternatives rejected

- **Real job queue / Redis** — explicitly rejected for a single-user scenario; goroutine + `sync.Map` suffices.
- **Persisting job history** — out of scope; jobs are in-memory only.

## Consequences

- Minimal infra, simple recovery.
- No durability: job state is lost on restart/crash. No cancellation.
- `context.Background()` is deliberate — the job outlives the request and is **not** cancelled if the client disconnects.
- The global mutex serializes imports system-wide.
- The WebSocket Hub / `Notifier`-interface conventions this builds on are in `docs/conventions/websocket.md`.

## Update 2026-10-11

The decision stands (in memory, no queue, one Job at a time), but the mechanism changed: Import, Backfill and Enrichment Jobs now all run on `service.JobRunner`, which keeps **only the latest Job** of its lane instead of a `sync.Map` of every Job. `GET /import-jobs/:id` therefore finds only the latest Import Job.
