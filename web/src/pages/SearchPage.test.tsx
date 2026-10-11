import { describe, it, expect, vi, beforeEach } from 'vitest'
import { fireEvent, screen } from '@testing-library/react'
import { renderWithRouter } from '../test/renderWithRouter'
import { listVideos } from '../api/videos'
import SearchPage from './SearchPage'

vi.mock('../api/videos', () => ({ listVideos: vi.fn() }))
vi.mock('../api/tags', () => ({ listTags: vi.fn(() => Promise.resolve([])) }))
vi.mock('../contexts/AuthContext', () => ({
  useAuth: () => ({ user: { username: 'steven', role: 'admin' }, isAdmin: true, logout: vi.fn() }),
}))

describe('SearchPage', () => {
  beforeEach(() => vi.clearAllMocks())

  it('says the search failed instead of showing no results, and retries', async () => {
    vi.mocked(listVideos)
      .mockRejectedValueOnce(new Error('down'))
      .mockResolvedValueOnce({ data: [], total: 0, page: 1, page_size: 40 } as never)
    renderWithRouter(<SearchPage />, { route: '/search?q=abc' })

    expect(await screen.findByRole('alert')).toHaveTextContent('搜尋失敗')
    expect(screen.queryByText(/沒有符合/)).toBeNull()

    fireEvent.click(screen.getByText('重試'))
    expect(await screen.findByText(/沒有符合「abc」/)).toBeInTheDocument()
  })
})
