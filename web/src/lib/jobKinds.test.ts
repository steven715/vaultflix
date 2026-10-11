import { describe, expect, it, vi } from 'vitest'
import { backfillJobs, importJobs } from './jobKinds'
import * as admin from '../api/admin'
import type { BackfillJob, ImportJob } from '../types'

vi.mock('../api/admin')

const importJob: ImportJob = {
  id: 'i1', source_id: 's1', source_label: 'D', status: 'completed', total: 3, processed: 3,
  imported: 1, skipped: 1, failed: 1, errors: [{ file_name: 'c.mkv', error: 'boom' }], started_at: '',
}

describe('importJobs', () => {
  it('maps the job onto the common view, keeping its Media Source', () => {
    expect(importJobs.finishedView(importJob)).toEqual({
      id: 'i1', phase: 'completed', subject: 's1', currentItem: '',
      counts: { total: 3, processed: 3, succeeded: 1, skipped: 1, failed: 1 },
      errors: [{ name: 'c.mkv', error: 'boom' }],
    })
  })

  it('turns progress into item started / done events with the server counts', () => {
    expect(importJobs.progressEvent({ job_id: 'i1', file_name: 'a.mkv', current: 1, total: 3, status: 'processing' }))
      .toEqual({ type: 'item-started', jobId: 'i1', item: 'a.mkv' })
    expect(importJobs.progressEvent({ job_id: 'i1', file_name: 'c.mkv', current: 3, total: 3, status: 'error', error: 'boom', imported: 1, skipped: 1, failed: 1 }))
      .toEqual({
        type: 'item-done', jobId: 'i1',
        counts: { total: 3, processed: 3, succeeded: 1, skipped: 1, failed: 1 },
        error: { name: 'c.mkv', error: 'boom' },
      })
  })

  it('starts on a Media Source', async () => {
    vi.mocked(admin.importVideos).mockResolvedValue({ ...importJob, status: 'running', processed: 0, imported: 0, skipped: 0, failed: 0, errors: [] })
    const v = await importJobs.start('s1')
    expect(admin.importVideos).toHaveBeenCalledWith('s1')
    expect(v.phase).toBe('running')
    expect(v.subject).toBe('s1')
  })

  it('cancels through the API', async () => {
    await importJobs.cancel?.('i1')
    expect(admin.cancelImportJob).toHaveBeenCalledWith('i1')
  })
})

describe('backfillJobs', () => {
  const job: BackfillJob = {
    id: 'b1', kind: 'codec', status: 'cancelled', total: 5, processed: 2, succeeded: 1, failed: 0, skipped: 1,
    errors: [], started_at: '',
  }

  it('maps the job and its Backfill kind', () => {
    expect(backfillJobs.finishedView(job)).toEqual({
      id: 'b1', phase: 'cancelled', subject: 'codec', currentItem: '',
      counts: { total: 5, processed: 2, succeeded: 1, skipped: 1, failed: 0 }, errors: [],
    })
  })

  it('starts a kind as a running job', async () => {
    vi.mocked(admin.startBackfill).mockResolvedValue({ job_id: 'b2', kind: 'keyframe' })
    expect(await backfillJobs.start('keyframe')).toEqual({
      id: 'b2', phase: 'running', subject: 'keyframe', currentItem: '',
      counts: { total: 0, processed: 0, succeeded: 0, skipped: 0, failed: 0 }, errors: [],
    })
  })

  it('cancels through the API', async () => {
    await backfillJobs.cancel?.('b1')
    expect(admin.cancelBackfill).toHaveBeenCalledWith('b1')
  })
})
