import { act, renderHook, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { useJob } from './useJob'
import type { JobKind, JobView } from '../lib/jobProgress'
import { wsHarness } from '../test/wsHarness'

const view = (id: string, over: Partial<JobView> = {}): JobView => ({
  id,
  phase: 'running',
  counts: { total: 4, processed: 0, succeeded: 0, skipped: 0, failed: 0 },
  currentItem: '',
  errors: [],
  ...over,
})

function fakeKind(over: Partial<JobKind<string>> = {}): JobKind<string> {
  return {
    messages: { progress: 'p', complete: 'c', error: 'e' },
    start: vi.fn(async () => view('new')),
    active: vi.fn(async () => null),
    progressEvent: (payload) => {
      const p = payload as { job_id: string; processed: number }
      return { type: 'item-done', jobId: p.job_id, counts: { total: 4, processed: p.processed, succeeded: p.processed, skipped: 0, failed: 0 } }
    },
    finishedView: (payload) => view((payload as { id: string }).id, { phase: 'completed' }),
    errorJobId: (payload) => (payload as { job_id: string }).job_id,
    ...over,
  }
}

describe('useJob', () => {
  it('adopts a running job on mount', async () => {
    const ws = wsHarness()
    const kind = fakeKind({ active: vi.fn(async () => view('j1')) })
    const { result } = renderHook(() => useJob(kind), { wrapper: ws.Provider })
    await waitFor(() => expect(result.current.view?.id).toBe('j1'))
  })

  it('does not adopt a finished job on mount', async () => {
    const ws = wsHarness()
    const kind = fakeKind({ active: vi.fn(async () => view('old', { phase: 'completed' })) })
    const { result } = renderHook(() => useJob(kind), { wrapper: ws.Provider })
    await waitFor(() => expect(kind.active).toHaveBeenCalled())
    expect(result.current.view).toBeNull()
  })

  it('applies progress for its job and ignores other kinds and jobs', async () => {
    const ws = wsHarness()
    const { result } = renderHook(() => useJob(fakeKind()), { wrapper: ws.Provider })
    await act(async () => { await result.current.start('src') })
    ws.send({ type: 'p', payload: { job_id: 'new', processed: 2 } })
    ws.send({ type: 'other', payload: { job_id: 'new', processed: 3 } })
    ws.send({ type: 'p', payload: { job_id: 'someone-else', processed: 4 } })
    await waitFor(() => expect(result.current.view?.counts.processed).toBe(2))
  })

  it('reports a started job, and adopts the running one on 409', async () => {
    const ws = wsHarness()
    const conflict = laneBusy()
    const kind = fakeKind({
      start: vi.fn(async () => { throw conflict }),
      active: vi.fn().mockResolvedValueOnce(null).mockResolvedValue(view('running-one')),
    })
    const { result } = renderHook(() => useJob(kind), { wrapper: ws.Provider })
    let outcome = ''
    await act(async () => { outcome = (await result.current.start('src')).outcome })
    expect(outcome).toBe('adopted')
    expect(result.current.view?.id).toBe('running-one')
  })

  // Another 409, such as a disabled Media Source, is that start's own failure,
  // even while some other job happens to run.
  it('fails, without adopting the running job, on a 409 other than a busy lane', async () => {
    const ws = wsHarness()
    const disabled = Object.assign(new Error('409'), {
      response: { status: 409, data: { error: 'source_disabled', message: 'media source is currently disabled' } },
    })
    const kind = fakeKind({
      start: vi.fn(async () => { throw disabled }),
      active: vi.fn().mockResolvedValueOnce(null).mockResolvedValue(view('running-one')),
    })
    const { result } = renderHook(() => useJob(kind), { wrapper: ws.Provider })
    let res: { outcome: string; error?: unknown } = { outcome: '' }
    await act(async () => { res = await result.current.start('src') })
    expect(res).toEqual({ outcome: 'failed', error: disabled })
    expect(result.current.view).toBeNull()
  })

  it('does not adopt a job that ended right after the 409', async () => {
    const ws = wsHarness()
    const conflict = laneBusy()
    const kind = fakeKind({
      start: vi.fn(async () => { throw conflict }),
      active: vi.fn().mockResolvedValueOnce(null).mockResolvedValue(view('gone', { phase: 'completed' })),
    })
    const onFinished = vi.fn()
    const { result } = renderHook(() => useJob(kind, { onFinished }), { wrapper: ws.Provider })
    let res: { outcome: string; error?: unknown } = { outcome: '' }
    await act(async () => { res = await result.current.start('src') })
    expect(res).toEqual({ outcome: 'failed', error: conflict })
    expect(result.current.view).toBeNull()
    expect(onFinished).not.toHaveBeenCalled()
  })

  it('shows the end of a job that finished before its start request returned', async () => {
    const ws = wsHarness()
    let resolveStart: (v: JobView) => void = () => {}
    const kind = fakeKind({ start: vi.fn(() => new Promise<JobView>((r) => { resolveStart = r })) })
    const { result } = renderHook(() => useJob(kind), { wrapper: ws.Provider })
    let pending: Promise<unknown> = Promise.resolve()
    act(() => { pending = result.current.start('src') })
    ws.send({ type: 'c', payload: { id: 'new' } })
    await act(async () => { resolveStart(view('new')); await pending })
    expect(result.current.view?.phase).toBe('completed')
  })

  it('sends one start request while one is in flight', async () => {
    const ws = wsHarness()
    const kind = fakeKind()
    const { result } = renderHook(() => useJob(kind), { wrapper: ws.Provider })
    await act(async () => { await Promise.all([result.current.start('src'), result.current.start('src')]) })
    expect(kind.start).toHaveBeenCalledTimes(1)
  })

  it('reports other start failures', async () => {
    const ws = wsHarness()
    const kind = fakeKind({ start: vi.fn(async () => { throw new Error('500') }) })
    const { result } = renderHook(() => useJob(kind), { wrapper: ws.Provider })
    let res: { outcome: string; error?: unknown } = { outcome: '' }
    await act(async () => { res = await result.current.start('src') })
    expect(res.outcome).toBe('failed')
    expect((res.error as Error).message).toBe('500')
    expect(result.current.view).toBeNull()
  })

  it('calls onFinished once when its job completes', async () => {
    const ws = wsHarness()
    const onFinished = vi.fn()
    const { result } = renderHook(() => useJob(fakeKind(), { onFinished }), { wrapper: ws.Provider })
    await act(async () => { await result.current.start('src') })
    ws.send({ type: 'c', payload: { id: 'new' } })
    ws.send({ type: 'p', payload: { job_id: 'new', processed: 4 } })
    await waitFor(() => expect(result.current.view?.phase).toBe('completed'))
    expect(onFinished).toHaveBeenCalledTimes(1)
  })

  it('fails only on its own job error', async () => {
    const ws = wsHarness()
    const { result } = renderHook(() => useJob(fakeKind()), { wrapper: ws.Provider })
    await act(async () => { await result.current.start('src') })
    ws.send({ type: 'e', payload: { job_id: 'another' } })
    await waitFor(() => expect(result.current.view?.phase).toBe('running'))
    ws.send({ type: 'e', payload: { job_id: 'new' } })
    await waitFor(() => expect(result.current.view?.phase).toBe('failed'))
  })

  it('offers cancel only for kinds that support it', async () => {
    const ws = wsHarness()
    const cancel = vi.fn(async () => {})
    const { result } = renderHook(() => useJob(fakeKind({ cancel })), { wrapper: ws.Provider })
    await act(async () => { await result.current.start('src') })
    await act(async () => { await result.current.cancel?.() })
    expect(cancel).toHaveBeenCalledWith('new')

    const plain = renderHook(() => useJob(fakeKind()), { wrapper: wsHarness().Provider })
    expect(plain.result.current.cancel).toBeUndefined()
  })

  it('dismiss clears the job', async () => {
    const ws = wsHarness()
    const { result } = renderHook(() => useJob(fakeKind()), { wrapper: ws.Provider })
    await act(async () => { await result.current.start('src') })
    act(() => result.current.dismiss())
    expect(result.current.view).toBeNull()
  })
})

function laneBusy() {
  return Object.assign(new Error('409'), {
    response: { status: 409, data: { error: 'job_lane_busy', message: '匯入正在執行，完成或取消後再試' } },
  })
}
