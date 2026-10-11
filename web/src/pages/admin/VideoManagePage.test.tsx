import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithRouter } from '../../test/renderWithRouter'
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
})
