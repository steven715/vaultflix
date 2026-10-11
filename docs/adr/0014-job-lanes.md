# ADR-0014: Jobs run on in-memory Job Lanes that refuse, not queue

- Status: Accepted (supersedes ADR-0003)
- Date: 2026-10-11

## Context

ADR-0003 made Import an in-memory background goroutine, one at a time. Since then the admin has gained two more kinds of Job (GLOSSARY.md「Job」): Backfill Jobs (four kinds) and Enrichment Jobs. Each kind hand-rolled the same lifecycle, and the three had drifted. Import kept every Job in a map forever and could not be cancelled; Enrichment copied the Backfill runner.

The Job kinds use different resources:

- Import and Backfill Jobs read the local disks and run ffmpeg/ffprobe. Both write the same Videos; for example, a Preview Backfill can reach a Video while an Import is creating it.
- Enrichment Jobs only call external websites, which rate-limit and can block.

Until now Import and Backfill could run at the same time.

## Decision

- **One runner.** `service.JobRunner` owns the lifecycle. It runs one Job at a time, keeps only the latest Job in memory, cancels between items, and reports every item as succeeded, skipped or failed. A Job kind supplies only its items and how to report them.
- **Job Lanes.** A lane is one runner instance:
  - The **Media Lane** is shared by Import and Backfill Jobs.
  - The **Enrichment Lane** holds Enrichment Jobs.
  - Different lanes run in parallel.
- **Refuse, don't queue.** Starting a Job on a busy lane fails with 409, and the message names the Job holding the lane (`JobLaneBusyError`). The next Job may start only after the previous one has sent its complete message.
- **Import Jobs can be cancelled** between files (`POST /api/import-jobs/:id/cancel`). Files already imported stay registered, and the next Import skips them.
- Jobs remain in memory only, with no queue and no persistence, as in ADR-0003.

## Alternatives rejected

- **One lane for every Job.** Enrichment would wait behind a long Import for no shared resource.
- **A lane per kind** (the old behaviour for Import and Backfill). Two ffmpeg-heavy Jobs would compete for CPU and disk, and both could write the same Video.
- **Queueing the refused Job.** A queue needs persistence or visible queue state to be trustworthy. At single-user scale (ADR-0009), "try again when it finishes" is simpler and is what the admin UI already handles.
- **Keeping every Job's history.** Nothing reads it: the UI restores only a running Job, and integration tests poll the Job they just started.

## Consequences

- The admin cannot start a Backfill while an Import runs, or the reverse. The toast says which Job holds the lane.
- `GET /api/import-jobs/:id` finds only the latest Import Job of the Media Lane.
- Job state is still lost on restart.
- A new Job kind needs only items and a reporter, and its frontend `JobKind` in `web/src/lib/jobKinds.ts`. Unifying the three API and WebSocket shapes is left to a later change.
