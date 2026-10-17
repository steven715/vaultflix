import { describe, it, expect, vi, beforeEach } from 'vitest'
import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithRouter } from '../../test/renderWithRouter'
import { wsHarness } from '../../test/wsHarness'
import { listVideos } from '../../api/videos'
import { addVideoTag, deleteVideo, getActiveBackfill, getActiveImportJob, importVideos, listMediaSources, startBackfill } from '../../api/admin'
import { listTags } from '../../api/tags'
import type { ImportJob } from '../../types'
import VideoManagePage from './VideoManagePage'

vi.mock('../../api/videos', () => ({
  listVideos: vi.fn(() => Promise.resolve({
    data: [
      { id: 'v1', title: '片A', description: '', thumbnail_url: undefined, duration_seconds: 600, file_size_bytes: 1024, resolution: '1080p', original_filename: 'a.mp4', tags: [], minio_object_key: '', thumbnail_key: '', preview_key: '', mime_type: '', created_at: '2026-01-01T00:00:00Z', updated_at: '' },
    ], total: 1, page: 1, page_size: 20,
  })),
}))
vi.mock('../../api/tags', () => ({ listTags: vi.fn(() => Promise.resolve([])) }))
vi.mock('../../api/admin', () => ({
  importVideos: vi.fn(), updateVideo: vi.fn(), deleteVideo: vi.fn(),
  listMediaSources: vi.fn(() => Promise.resolve([])),
  getActiveImportJob: vi.fn(() => Promise.resolve(null)),
  startBackfill: vi.fn(), getActiveBackfill: vi.fn(() => Promise.resolve(null)),
  addVideoTag: vi.fn(), removeVideoTag: vi.fn(),
}))
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn(), info: vi.fn() }))
vi.mock('../../contexts/ToastContext', () => ({ useToast: () => toast }))

describe('VideoManagePage (library)', () => {
  beforeEach(() => vi.clearAllMocks())
  it('renders the video row and shows batch bar after selecting a row', async () => {
    renderWithRouter(<VideoManagePage />, { route: '/admin/library' })
    await waitFor(() => expect(screen.getByText('片A')).toBeInTheDocument())
    const checkbox = screen.getByLabelText('選取 片A')
    await userEvent.click(checkbox)
    expect(screen.getByText(/已選取/)).toBeInTheDocument()
  })

  const runningImport: ImportJob = {
    id: 'i1', source_id: 's1', source_label: 'D', status: 'running', total: 1, processed: 0,
    imported: 0, skipped: 0, failed: 0, errors: [], started_at: '',
  }

  // Start an Import from the dialog on route, then end it over WebSocket.
  async function importAndEnd(route: string, end: Partial<ImportJob>) {
    vi.mocked(importVideos).mockResolvedValue(runningImport)
    vi.mocked(listMediaSources).mockResolvedValue([
      { id: 's1', label: 'D槽', mount_path: '/mnt/D', enabled: true, video_count: 0, created_at: '', updated_at: '' },
    ])
    const ws = wsHarness()
    renderWithRouter(<VideoManagePage />, { route, ws })
    await screen.findByText('片A')
    fireEvent.click(screen.getByRole('button', { name: '匯入影片' }))
    await screen.findByRole('option', { name: /D槽/ })
    fireEvent.click(screen.getByRole('button', { name: '開始匯入' }))
    await waitFor(() => expect(importVideos).toHaveBeenCalledWith('s1'))
    const loads = vi.mocked(listVideos).mock.calls.length
    ws.send({ type: 'import_complete', payload: { ...runningImport, processed: 1, ...end } })
    return loads
  }

  it('reloads the list when an Import ends while already on page 1', async () => {
    const loads = await importAndEnd('/admin/library', { status: 'completed', imported: 1 })
    await waitFor(() => expect(vi.mocked(listVideos).mock.calls.length).toBe(loads + 1))
  })

  it('shows page 1, loaded once, when an Import ends on a later page', async () => {
    const loads = await importAndEnd('/admin/library?page=3', { status: 'completed', imported: 1 })
    await waitFor(() => expect(vi.mocked(listVideos).mock.calls.at(-1)?.[0]).toMatchObject({ page: 1 }))
    await new Promise((r) => setTimeout(r, 50))
    expect(vi.mocked(listVideos).mock.calls.length).toBe(loads + 1)
  })

  it('stays on the page when an Import imported nothing', async () => {
    const loads = await importAndEnd('/admin/library?page=3', { status: 'failed', failed: 1 })
    await screen.findByText('匯入失敗')
    await new Promise((r) => setTimeout(r, 50))
    expect(vi.mocked(listVideos).mock.calls.length).toBe(loads)
  })

  const backfillJob = (status: string) => ({
    id: 'b1', kind: 'keyframe', status, total: 0, processed: 0, succeeded: 0, failed: 0, skipped: 0, errors: [], started_at: '',
  })

  it('reloads the list on the same page when a Backfill Job ends', async () => {
    vi.mocked(startBackfill).mockResolvedValue({ job_id: 'b1', kind: 'keyframe' })
    // Page 2 exists: 21 Videos, 20 per page.
    vi.mocked(listVideos).mockResolvedValue({
      data: [{ id: 'v1', title: '片A', description: '', duration_seconds: 600, file_size_bytes: 1024, resolution: '1080p', original_filename: 'a.mp4', tags: [], minio_object_key: '', thumbnail_key: '', preview_key: '', mime_type: '', created_at: '2026-01-01T00:00:00Z', updated_at: '' }],
      total: 21, page: 2, page_size: 20,
    } as never)
    const ws = wsHarness()
    renderWithRouter(<VideoManagePage />, { route: '/admin/library?page=2', ws })
    await screen.findByText('片A')

    fireEvent.click(screen.getByRole('button', { name: '補齊 ▾' }))
    fireEvent.click(screen.getByRole('menuitem', { name: 'Keyframe Index' }))
    await waitFor(() => expect(startBackfill).toHaveBeenCalledWith('keyframe'))
    const loads = vi.mocked(listVideos).mock.calls.length
    // The reload hangs: the rows must not wait for it.
    vi.mocked(listVideos).mockImplementationOnce(() => new Promise(() => {}))

    ws.send({ type: 'backfill_complete', payload: backfillJob('completed') })
    await waitFor(() => expect(vi.mocked(listVideos).mock.calls.length).toBe(loads + 1))
    // A quiet reload: the rows stay while it loads.
    expect(screen.queryByText('載入中...')).toBeNull()
    expect(screen.getByText('片A')).toBeInTheDocument()
    expect(vi.mocked(listVideos).mock.calls.at(-1)?.[0]).toMatchObject({ page: 2 })
  })

  it('disables 補齊 while an Import holds the Media Lane', async () => {
    vi.mocked(getActiveImportJob).mockResolvedValue({
      id: 'i1', source_id: 's1', source_label: 'D', status: 'running', total: 3, processed: 1,
      imported: 1, skipped: 0, failed: 0, errors: [], started_at: '',
    })
    renderWithRouter(<VideoManagePage />, { route: '/admin/library' })

    await waitFor(() => expect(screen.getByRole('button', { name: '補齊 ▾' })).toBeDisabled())
    expect(screen.getByRole('button', { name: '補齊 ▾' })).toHaveAttribute('title', '匯入執行中，完成或取消後再補齊')
  })

  it('disables 開始匯入 while a Backfill holds the Media Lane', async () => {
    vi.mocked(getActiveBackfill).mockResolvedValue(backfillJob('running') as never)
    vi.mocked(listMediaSources).mockResolvedValue([
      { id: 's1', label: 'D槽', mount_path: '/mnt/D', enabled: true, video_count: 0, created_at: '', updated_at: '' },
    ])
    renderWithRouter(<VideoManagePage />, { route: '/admin/library' })
    await screen.findByText(/補齊 Keyframe Index 中/)

    fireEvent.click(screen.getByRole('button', { name: '匯入影片' }))
    await screen.findByRole('option', { name: /D槽/ })
    expect(screen.getByRole('button', { name: '開始匯入' })).toBeDisabled()
    expect(screen.getByText('補齊執行中，完成或取消後再匯入')).toBeInTheDocument()
  })

  it('offers 關閉 on the Backfill panel only once the job has ended', async () => {
    vi.mocked(getActiveBackfill).mockResolvedValue(backfillJob('running') as never)
    renderWithRouter(<VideoManagePage />, { route: '/admin/library' })
    await screen.findByText(/補齊 Keyframe Index 中/)
    expect(screen.queryByRole('button', { name: '關閉' })).toBeNull()
  })

  // Sorting by a column changes the whole order: staying on page 3 would show
  // an arbitrary slice of it.
  it('goes back to page 1 when sorting by a column', async () => {
    renderWithRouter(<VideoManagePage />, { route: '/admin/library?page=3' })
    await screen.findByText('片A')
    expect(listVideos).toHaveBeenLastCalledWith(expect.objectContaining({ page: 3 }))

    fireEvent.click(screen.getByRole('columnheader', { name: /影片/ }))

    await waitFor(() => expect(listVideos).toHaveBeenLastCalledWith(
      expect.objectContaining({ page: 1, sort_by: 'title', sort_order: 'desc' }),
    ))
  })
  // Two Videos on the page, both selected.
  async function selectTwo() {
    const row = (id: string, title: string) => ({
      id, title, description: '', duration_seconds: 600, file_size_bytes: 1024, resolution: '1080p', original_filename: `${id}.mp4`,
      tags: [], minio_object_key: '', thumbnail_key: '', preview_key: '', mime_type: '', created_at: '2026-01-01T00:00:00Z', updated_at: '',
    })
    vi.mocked(listVideos).mockResolvedValue({ data: [row('v1', '片A'), row('v2', '片B')], total: 2, page: 1, page_size: 20 } as never)
    renderWithRouter(<VideoManagePage />, { route: '/admin/library' })
    await userEvent.click(await screen.findByLabelText('選取 片A'))
    await userEvent.click(screen.getByLabelText('選取 片B'))
  }

  // A failure in a batch must not read as done: say how many failed, and
  // keep those selected so the admin can retry them.
  it('says how many Videos a batch delete could not delete, and keeps them selected', async () => {
    vi.mocked(deleteVideo).mockImplementation(async (id: string) => { if (id === 'v2') throw new Error('500') })
    await selectTwo()

    const batchBar = screen.getByText(/已選取/).closest('div')!
    fireEvent.click(within(batchBar).getByText('刪除', { selector: 'button' }))

    await waitFor(() => expect(toast.error).toHaveBeenCalledWith('2 部中有 1 部刪除失敗'))
    expect(screen.getByLabelText('選取 片B')).toBeChecked()
    expect(screen.getByLabelText('選取 片A')).not.toBeChecked()
  })

  it('says how many Videos a batch tag could not tag, instead of done', async () => {
    vi.mocked(listTags).mockResolvedValue([{ id: 7, name: '動作', category: 'genre', video_count: 1 }] as never)
    vi.mocked(addVideoTag).mockImplementation(async (id: string) => { if (id === 'v1') throw new Error('500') })
    await selectTwo()

    fireEvent.click(screen.getByText('批次加標籤', { selector: 'button' }))
    fireEvent.click(await screen.findByText('套用'))

    await waitFor(() => expect(toast.error).toHaveBeenCalledWith('2 部中有 1 部加標籤失敗'))
    expect(toast.success).not.toHaveBeenCalled()
  })
  // Deleting the last Video of the last page leaves a page past the end:
  // show the page that is now last instead of an empty list.
  it('moves to the new last page when a delete empties the current one', async () => {
    const only = {
      id: 'v21', title: '第21部', description: '', duration_seconds: 600, file_size_bytes: 1024, resolution: '1080p', original_filename: 'v21.mp4',
      tags: [], minio_object_key: '', thumbnail_key: '', preview_key: '', mime_type: '', created_at: '2026-01-01T00:00:00Z', updated_at: '',
    }
    vi.mocked(listVideos).mockResolvedValueOnce({ data: [only], total: 21, page: 2, page_size: 20 } as never)
    vi.mocked(deleteVideo).mockResolvedValue(undefined)
    renderWithRouter(<VideoManagePage />, { route: '/admin/library?page=2' })
    const row = (await screen.findByText('第21部')).closest('tr')!

    fireEvent.click(within(row).getByText('刪除', { selector: 'button' }))
    const buttons = screen.getAllByText('刪除', { selector: 'button' })
    fireEvent.click(buttons[buttons.length - 1])

    await waitFor(() => expect(listVideos).toHaveBeenLastCalledWith(expect.objectContaining({ page: 1 })))
  })
})
