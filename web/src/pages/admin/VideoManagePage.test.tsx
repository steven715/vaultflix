import { describe, it, expect, vi, beforeEach } from 'vitest'
import { fireEvent, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithRouter } from '../../test/renderWithRouter'
import { wsHarness } from '../../test/wsHarness'
import { listVideos } from '../../api/videos'
import { getActiveBackfill, getActiveImportJob, listMediaSources, startBackfill } from '../../api/admin'
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
vi.mock('../../contexts/ToastContext', () => ({ useToast: () => ({ success: vi.fn(), error: vi.fn(), info: vi.fn() }) }))

describe('VideoManagePage (library)', () => {
  beforeEach(() => vi.clearAllMocks())
  it('renders the video row and shows batch bar after selecting a row', async () => {
    renderWithRouter(<VideoManagePage />, { route: '/admin/library' })
    await waitFor(() => expect(screen.getByText('片A')).toBeInTheDocument())
    const checkbox = screen.getByLabelText('選取 片A')
    await userEvent.click(checkbox)
    expect(screen.getByText(/已選取/)).toBeInTheDocument()
  })

  const backfillJob = (status: string) => ({
    id: 'b1', kind: 'keyframe', status, total: 0, processed: 0, succeeded: 0, failed: 0, skipped: 0, errors: [], started_at: '',
  })

  it('reloads the list on the same page when a Backfill Job ends', async () => {
    vi.mocked(startBackfill).mockResolvedValue({ job_id: 'b1', kind: 'keyframe' })
    const ws = wsHarness()
    renderWithRouter(<VideoManagePage />, { route: '/admin/library?page=2', ws })
    await screen.findByText('片A')

    fireEvent.click(screen.getByRole('button', { name: '補齊 ▾' }))
    fireEvent.click(screen.getByRole('menuitem', { name: 'Keyframe Index' }))
    await waitFor(() => expect(startBackfill).toHaveBeenCalledWith('keyframe'))
    const loads = vi.mocked(listVideos).mock.calls.length

    ws.send({ type: 'backfill_complete', payload: backfillJob('completed') })
    await waitFor(() => expect(vi.mocked(listVideos).mock.calls.length).toBe(loads + 1))
    expect(vi.mocked(listVideos).mock.calls.at(-1)?.[0]).toMatchObject({ page: 2 })
  })

  it('disables 補齊 while an Import holds the Media Lane', async () => {
    vi.mocked(getActiveImportJob).mockResolvedValue({
      id: 'i1', source_id: 's1', source_label: 'D', status: 'running', total: 3, processed: 1,
      imported: 1, skipped: 0, failed: 0, errors: [], started_at: '',
    })
    renderWithRouter(<VideoManagePage />, { route: '/admin/library' })

    await waitFor(() => expect(screen.getByRole('button', { name: '補齊 ▾' })).toBeDisabled())
    expect(screen.getByRole('button', { name: '補齊 ▾' })).toHaveAttribute('title', '匯入執行中')
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
})
