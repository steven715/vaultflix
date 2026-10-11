import { describe, expect, it } from 'vitest'
import { reduceJob, type JobView } from './jobProgress'

const running: JobView = {
  id: 'j1',
  phase: 'running',
  counts: { total: 10, processed: 2, succeeded: 2, skipped: 0, failed: 0 },
  currentItem: '',
  errors: [],
}

describe('reduceJob', () => {
  it('replaces counts with the server numbers from each result', () => {
    const next = reduceJob(running, {
      type: 'item-done', jobId: 'j1',
      counts: { total: 10, processed: 5, succeeded: 3, skipped: 1, failed: 1 },
      error: { name: 'c.mkv', error: 'boom' },
    })
    expect(next?.counts).toEqual({ total: 10, processed: 5, succeeded: 3, skipped: 1, failed: 1 })
    expect(next?.errors).toEqual([{ name: 'c.mkv', error: 'boom' }])
  })

  it('shows the item being processed', () => {
    expect(reduceJob(running, { type: 'item-started', jobId: 'j1', item: 'd.mkv' })?.currentItem).toBe('d.mkv')
  })

  it('ignores events of another job, and events before any job is known', () => {
    const other = { type: 'item-done' as const, jobId: 'j2', counts: { ...running.counts, processed: 9 } }
    expect(reduceJob(running, other)).toBe(running)
    expect(reduceJob(null, other)).toBeNull()
    expect(reduceJob(running, { type: 'failed', jobId: 'j2' })).toBe(running)
  })

  it('never reopens a finished job from a late running snapshot', () => {
    const done: JobView = { ...running, phase: 'completed' }
    expect(reduceJob(done, { type: 'snapshot', view: running })).toBe(done)
  })

  it('ignores a finish when no job is shown', () => {
    expect(reduceJob(null, { type: 'finished', view: { ...running, phase: 'completed' } })).toBeNull()
  })

  it('takes the final job on finish', () => {
    const done: JobView = { ...running, phase: 'completed', counts: { ...running.counts, processed: 10, succeeded: 10 } }
    expect(reduceJob(running, { type: 'finished', view: done })).toEqual(done)
  })

  it('marks the job failed on its own error', () => {
    expect(reduceJob(running, { type: 'failed', jobId: 'j1' })?.phase).toBe('failed')
  })

  // A REST snapshot can resolve after live results already moved the job on.
  it('a late snapshot never rolls the counts back', () => {
    const ahead = reduceJob(running, {
      type: 'item-done', jobId: 'j1', counts: { total: 10, processed: 6, succeeded: 6, skipped: 0, failed: 0 },
    })
    const late = reduceJob(ahead, { type: 'snapshot', view: running })
    expect(late?.counts.processed).toBe(6)
  })

  it('a snapshot of a new job replaces the old one', () => {
    const fresh: JobView = { ...running, id: 'j3' }
    expect(reduceJob(running, { type: 'snapshot', view: fresh })).toEqual(fresh)
  })

  it('reset clears the job', () => {
    expect(reduceJob(running, { type: 'reset' })).toBeNull()
  })
})
