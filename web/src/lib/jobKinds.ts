// The admin UI's job kinds: each maps its own API and WebSocket messages onto
// the common JobView / JobEvent shapes (jobProgress.ts). Adding a kind (e.g.
// the Enrichment batch) means adding one object here, not another component.
import { cancelBackfill, cancelImportJob, getActiveBackfill, getActiveImportJob, importVideos, startBackfill } from '../api/admin'
import type { BackfillJob, BackfillKind, BackfillProgress, ImportJob, ImportProgress } from '../types'
import type { JobCounts, JobEvent, JobKind, JobView } from './jobProgress'

const noCounts: JobCounts = { total: 0, processed: 0, succeeded: 0, skipped: 0, failed: 0 }

function importView(job: ImportJob): JobView {
  return {
    id: job.id,
    phase: job.status,
    subject: job.source_id,
    currentItem: '',
    counts: { total: job.total, processed: job.processed, succeeded: job.imported, skipped: job.skipped, failed: job.failed },
    errors: (job.errors ?? []).map((e) => ({ name: e.file_name, error: e.error })),
  }
}

/** Import Jobs, started on a Media Source id. Cancelled between files. */
export const importJobs: JobKind<string> = {
  messages: { progress: 'import_progress', complete: 'import_complete', error: 'import_error' },
  start: async (sourceId) => importView(await importVideos(sourceId)),
  active: async () => {
    const job = await getActiveImportJob()
    return job ? importView(job) : null
  },
  cancel: (jobId) => cancelImportJob(jobId),
  progressEvent(payload): JobEvent {
    const p = payload as ImportProgress
    if (p.status === 'processing') return { type: 'item-started', jobId: p.job_id, item: p.file_name }
    return {
      type: 'item-done',
      jobId: p.job_id,
      counts: { total: p.total, processed: p.current, succeeded: p.imported, skipped: p.skipped, failed: p.failed },
      error: p.status === 'error' ? { name: p.file_name, error: p.error ?? '' } : undefined,
    }
  },
  finishedView: (payload) => importView(payload as ImportJob),
  errorJobId: (payload) => (payload as { job_id?: string }).job_id ?? null,
}

function backfillView(job: BackfillJob): JobView {
  return {
    id: job.id,
    phase: job.status,
    subject: job.kind,
    currentItem: '',
    counts: { total: job.total, processed: job.processed, succeeded: job.succeeded, skipped: job.skipped, failed: job.failed },
    errors: (job.errors ?? []).map((e) => ({ name: e.original_filename, error: e.error })),
  }
}

/** Backfill Jobs, started for one Backfill kind. Cancelled between Videos. */
export const backfillJobs: JobKind<BackfillKind> = {
  messages: { progress: 'backfill_progress', complete: 'backfill_complete', error: 'backfill_error' },
  start: async (kind) => {
    const { job_id } = await startBackfill(kind)
    return { id: job_id, phase: 'running', subject: kind, currentItem: '', counts: noCounts, errors: [] }
  },
  active: async () => {
    const job = await getActiveBackfill()
    return job ? backfillView(job) : null
  },
  cancel: (jobId) => cancelBackfill(jobId),
  progressEvent(payload): JobEvent {
    const p = payload as BackfillProgress
    if (p.status === 'processing') return { type: 'item-started', jobId: p.job_id, item: p.original_filename }
    return {
      type: 'item-done',
      jobId: p.job_id,
      counts: { total: p.total, processed: p.current, succeeded: p.succeeded, skipped: p.skipped, failed: p.failed },
      error: p.status === 'error' ? { name: p.original_filename, error: p.error ?? '' } : undefined,
    }
  },
  finishedView: (payload) => backfillView(payload as BackfillJob),
  errorJobId: (payload) => (payload as { job_id?: string }).job_id ?? null,
}
