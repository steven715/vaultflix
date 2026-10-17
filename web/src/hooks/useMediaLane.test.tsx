import { act, renderHook, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useMediaLane } from './useMediaLane'
import { wsHarness } from '../test/wsHarness'
import { getActiveBackfill, getActiveImportJob, importVideos, startBackfill } from '../api/admin'
import type { BackfillJob, ImportJob } from '../types'

vi.mock('../api/admin', () => ({
  importVideos: vi.fn(), getActiveImportJob: vi.fn(), cancelImportJob: vi.fn(),
  startBackfill: vi.fn(), getActiveBackfill: vi.fn(), cancelBackfill: vi.fn(),
}))
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn(), info: vi.fn() }))
vi.mock('../contexts/ToastContext', () => ({ useToast: () => toast }))

const runningImport: ImportJob = {
  id: 'i1', source_id: 's1', source_label: 'D', status: 'running', total: 3, processed: 1,
  imported: 1, skipped: 0, failed: 0, errors: [], started_at: '',
}
const runningBackfill: BackfillJob = {
  id: 'b1', kind: 'keyframe', status: 'running', total: 4, processed: 1, succeeded: 1, failed: 0, skipped: 0, errors: [], started_at: '',
}

function lane(opts: Parameters<typeof useMediaLane>[0] = {}) {
  const ws = wsHarness()
  return { ws, ...renderHook(() => useMediaLane(opts), { wrapper: ws.Provider }) }
}

describe('useMediaLane', () => {
  beforeEach(() => {
    vi.mocked(getActiveImportJob).mockResolvedValue(null)
    vi.mocked(getActiveBackfill).mockResolvedValue(null)
  })

  it('leaves both Jobs free to start on an idle lane', async () => {
    const { result } = lane()
    await waitFor(() => expect(getActiveBackfill).toHaveBeenCalled())
    expect(result.current.importBlockedReason).toBeNull()
    expect(result.current.backfillBlockedReason).toBeNull()
  })

  it('says a running Import holds the lane', async () => {
    vi.mocked(getActiveImportJob).mockResolvedValue(runningImport)
    const { result } = lane()
    await waitFor(() => expect(result.current.backfillBlockedReason).toBe('匯入執行中，完成或取消後再補齊'))
    expect(result.current.importBlockedReason).toBe('匯入執行中')
  })

  it('says a running Backfill holds the lane', async () => {
    vi.mocked(getActiveBackfill).mockResolvedValue(runningBackfill)
    const { result } = lane()
    await waitFor(() => expect(result.current.importBlockedReason).toBe('補齊執行中，完成或取消後再匯入'))
    expect(result.current.backfillBlockedReason).toBe('補齊執行中')
  })

  it('locks the other Job while a start is in flight', async () => {
    vi.mocked(startBackfill).mockReturnValue(new Promise(() => {}))
    const { result } = lane()
    act(() => { void result.current.backfill.start('keyframe') })
    await waitFor(() => expect(result.current.importBlockedReason).toBe('補齊執行中，完成或取消後再匯入'))
  })

  it('toasts the server message when a start is refused', async () => {
    const busy = Object.assign(new Error('409'), {
      isAxiosError: true,
      response: { status: 409, data: { error: 'job_lane_busy', message: '匯入正在執行，完成或取消後再試' } },
    })
    vi.mocked(startBackfill).mockRejectedValue(busy)
    const { result } = lane()
    let outcome = ''
    await act(async () => { outcome = await result.current.backfill.start('codec') })
    expect(outcome).toBe('failed')
    expect(toast.error).toHaveBeenCalledWith('匯入正在執行，完成或取消後再試')
  })

  it('reports each Job ending to the page', async () => {
    vi.mocked(importVideos).mockResolvedValue(runningImport)
    const onImportFinished = vi.fn()
    const { result, ws } = lane({ onImportFinished })
    await act(async () => { await result.current.import.start('s1') })

    ws.send({ type: 'import_complete', payload: { ...runningImport, status: 'completed', processed: 3 } })
    await waitFor(() => expect(onImportFinished).toHaveBeenCalledTimes(1))
    expect(onImportFinished.mock.calls[0][0]).toMatchObject({ id: 'i1', phase: 'completed' })
  })

  it('toasts when a start finds the same kind already running elsewhere', async () => {
    const conflict = Object.assign(new Error('409'), {
      response: { status: 409, data: { error: 'job_lane_busy', message: '匯入正在執行，完成或取消後再試' } },
    })
    vi.mocked(importVideos).mockRejectedValue(conflict)
    vi.mocked(getActiveImportJob).mockResolvedValueOnce(null).mockResolvedValue(runningImport)
    const { result } = lane()
    let outcome = ''
    await act(async () => { outcome = await result.current.import.start('s1') })
    expect(outcome).toBe('adopted')
    expect(toast.error).toHaveBeenCalledWith('已有任務進行中')
  })

  it('says why an Import did not start', async () => {
    vi.mocked(importVideos).mockRejectedValue(Object.assign(new Error('409'), {
      response: { status: 409, data: { error: 'source_disabled', message: 'media source is currently disabled' } },
    }))
    const { result } = lane()
    await act(async () => { await result.current.import.start('s1') })
    expect(toast.error).toHaveBeenCalledWith('媒體來源已停用，啟用後再匯入')
  })

  it('says each Job is busy while it starts or runs', async () => {
    vi.mocked(getActiveBackfill).mockResolvedValue(runningBackfill)
    const { result } = lane()
    await waitFor(() => expect(result.current.backfill.busy).toBe(true))
    expect(result.current.import.busy).toBe(false)
  })

  it('keeps start and cancel the same functions across renders', async () => {
    const { result, rerender } = lane()
    const before = result.current
    rerender()
    expect(result.current.import.start).toBe(before.import.start)
    expect(result.current.backfill.cancel).toBe(before.backfill.cancel)
  })
})
