import { describe, it, expect, vi, beforeEach } from 'vitest'
import { fireEvent, screen, waitFor } from '@testing-library/react'
import { renderWithRouter } from '../test/renderWithRouter'
import { listVideos } from '../api/videos'
import BrowsePage from './BrowsePage'

vi.mock('../api/videos', () => ({ listVideos: vi.fn() }))
vi.mock('../api/recommendations', () => ({ getTodayRecommendations: vi.fn(() => Promise.resolve([])) }))
vi.mock('../api/watchHistory', () => ({ listWatchHistory: vi.fn(() => Promise.resolve({ data: [], total: 0, page: 1, page_size: 12 })) }))
vi.mock('../api/tags', () => ({ listTags: vi.fn(() => Promise.resolve([])) }))
vi.mock('../contexts/AuthContext', () => ({
  useAuth: () => ({ user: { username: 'steven', role: 'viewer' }, isAdmin: false, logout: vi.fn() }),
}))

const video = (id: string, title: string) => ({
  id, title, description: '', duration_seconds: 600, file_size_bytes: 1024, resolution: '1080p', original_filename: `${id}.mp4`,
  tags: [], minio_object_key: '', thumbnail_key: '', preview_key: '', mime_type: '', created_at: '2026-01-01T00:00:00Z', updated_at: '',
})

describe('BrowsePage', () => {
  beforeEach(() => vi.clearAllMocks())

  // A stale ?page= (an old bookmark, a shrunk Library) shows the last page, not an empty list.
  it('moves a page past the end to the last page', async () => {
    vi.mocked(listVideos).mockImplementation(async (p) => ({
      data: p.page === 1 ? [video('v1', '片A')] : [], total: 5, page: p.page ?? 1, page_size: 20,
    }) as never)
    renderWithRouter(<BrowsePage />, { route: '/?q=a&page=9' })

    await waitFor(() => expect(listVideos).toHaveBeenLastCalledWith(expect.objectContaining({ page: 1 })))
  })

  it('says the Library failed to load, and retries', async () => {
    vi.mocked(listVideos)
      .mockRejectedValueOnce(new Error('down'))
      .mockResolvedValue({ data: [video('v1', '片A')], total: 1, page: 1, page_size: 20 } as never)
    renderWithRouter(<BrowsePage />, { route: '/?q=a' })

    fireEvent.click(await screen.findByText('重試'))

    expect(await screen.findAllByText('片A')).not.toHaveLength(0)
  })
})
