import { describe, it, expect, vi, beforeEach } from 'vitest'
import { fireEvent, screen } from '@testing-library/react'
import { renderWithRouter } from '../test/renderWithRouter'
import { listFavorites, removeFavorite } from '../api/favorites'
import FavoritesPage from './FavoritesPage'

vi.mock('../api/favorites', () => ({ listFavorites: vi.fn(), removeFavorite: vi.fn() }))
vi.mock('../contexts/AuthContext', () => ({
  useAuth: () => ({ user: { username: 'steven', role: 'viewer' }, isAdmin: false, logout: vi.fn() }),
}))
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn(), info: vi.fn() }))
vi.mock('../contexts/ToastContext', () => ({ useToast: () => toast }))

const video = (id: string, title: string) => ({
  id, title, duration_seconds: 600, resolution: '1080p', file_size_bytes: 1024, thumbnail_url: undefined,
})

describe('FavoritesPage', () => {
  beforeEach(() => vi.clearAllMocks())

  // Removing the only favorite on the last page leaves a page past the end:
  // show the page that is now last, not "還沒有收藏".
  it('moves to the new last page when removing its only favorite', async () => {
    let total = 21
    vi.mocked(listFavorites).mockImplementation(async (page = 1) => ({
      data: page === 2 && total > 20 ? [video('v21', '第21部')] : [video('v1', '第1部')],
      total, page, page_size: 20,
    }) as never)
    vi.mocked(removeFavorite).mockImplementation(async () => { total = 20 })
    renderWithRouter(<FavoritesPage />, { route: '/favorites' })
    await screen.findByText('第1部', { selector: 'h3' })
    fireEvent.click(screen.getByText('2', { selector: 'button' }))
    await screen.findByText('第21部', { selector: 'h3' })

    fireEvent.click(screen.getByTitle('取消收藏'))

    expect(await screen.findByText('第1部', { selector: 'h3' })).toBeInTheDocument()
    expect(screen.queryByText('還沒有收藏')).toBeNull()
  })
  // The removal must reach the server before the list moves to another page,
  // or that page loads with the old total.
  it('moves pages only after the server has removed the favorite', async () => {
    let total = 21
    let finishRemove!: () => void
    vi.mocked(listFavorites).mockImplementation(async (page = 1) => ({
      data: page === 2 && total > 20 ? [video('v21', '第21部')] : [video('v1', '第1部')],
      total, page, page_size: 20,
    }) as never)
    vi.mocked(removeFavorite).mockImplementation(() => new Promise<void>((r) => { finishRemove = () => { total = 20; r() } }))
    renderWithRouter(<FavoritesPage />, { route: '/favorites' })
    await screen.findByText('第1部', { selector: 'h3' })
    fireEvent.click(screen.getByText('2', { selector: 'button' }))
    await screen.findByText('第21部', { selector: 'h3' })

    fireEvent.click(screen.getByTitle('取消收藏'))
    expect(screen.queryByText('第21部', { selector: 'h3' })).toBeNull()
    expect(listFavorites).toHaveBeenCalledTimes(2)

    finishRemove()
    expect(await screen.findByText('第1部', { selector: 'h3' })).toBeInTheDocument()
    expect(screen.queryByText('2', { selector: 'button' })).toBeNull()
  })
})
