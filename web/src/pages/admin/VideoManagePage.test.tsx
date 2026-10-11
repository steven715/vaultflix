import { describe, it, expect, vi, beforeEach } from 'vitest'
import { fireEvent, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithRouter } from '../../test/renderWithRouter'
import { wsHarness } from '../../test/wsHarness'
import { listVideos } from '../../api/videos'
import { importVideos, listMediaSources } from '../../api/admin'
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

  it('reloads the list when an Import ends while already on page 1', async () => {
    const job = {
      id: 'i1', source_id: 's1', source_label: 'D', status: 'running', total: 1, processed: 0,
      imported: 0, skipped: 0, failed: 0, errors: [], started_at: '',
    }
    vi.mocked(importVideos).mockResolvedValue(job as never)
    vi.mocked(listMediaSources).mockResolvedValue([
      { id: 's1', label: 'D槽', mount_path: '/mnt/D', enabled: true, video_count: 0, created_at: '', updated_at: '' },
    ])
    const ws = wsHarness()
    renderWithRouter(<VideoManagePage />, { route: '/admin/library', ws })
    await screen.findByText('片A')

    fireEvent.click(screen.getByRole('button', { name: '匯入影片' }))
    await screen.findByRole('option', { name: /D槽/ })
    fireEvent.click(screen.getByRole('button', { name: '開始匯入' }))
    await waitFor(() => expect(importVideos).toHaveBeenCalledWith('s1'))
    const loads = vi.mocked(listVideos).mock.calls.length

    ws.send({ type: 'import_complete', payload: { ...job, status: 'completed', processed: 1, imported: 1 } })
    await waitFor(() => expect(vi.mocked(listVideos).mock.calls.length).toBe(loads + 1))
  })
})
