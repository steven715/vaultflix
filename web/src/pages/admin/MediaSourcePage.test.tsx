import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import { renderWithRouter } from '../../test/renderWithRouter'
import MediaSourcePage from './MediaSourcePage'

vi.mock('../../api/admin', () => ({
  listMediaSources: vi.fn(() => Promise.resolve([
    { id: 's1', label: 'D槽', mount_path: '/mnt/host/D', enabled: true, video_count: 3, created_at: '', updated_at: '' },
  ])),
  getActiveImportJob: vi.fn(() => Promise.resolve(null)),
  createMediaSource: vi.fn(), updateMediaSource: vi.fn(), deleteMediaSource: vi.fn(), importVideos: vi.fn(),
}))
vi.mock('../../contexts/ToastContext', () => ({ useToast: () => ({ success: vi.fn(), error: vi.fn(), info: vi.fn() }) }))

describe('MediaSourcePage (reskin)', () => {
  beforeEach(() => vi.clearAllMocks())
  it('renders sources and uses design-token background (no embedded admin header)', async () => {
    const { container } = renderWithRouter(<MediaSourcePage />, { route: '/admin/media-sources' })
    await waitFor(() => expect(screen.getByText('D槽')).toBeInTheDocument())
    expect(screen.getByText('/mnt/host/D')).toBeInTheDocument()
    // 不再自帶搜尋框（那是 AdminTopbar 的職責）
    expect(screen.queryByPlaceholderText('搜尋影片...')).toBeNull()
    // 頁面根容器用 token，不再是 bg-gray-950
    expect(container.querySelector('.bg-gray-950')).toBeNull()
  })
})
