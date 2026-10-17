import { describe, it, expect, vi, beforeEach } from 'vitest'
import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import { renderWithRouter } from '../test/renderWithRouter'
import { clearWatchHistory, listWatchHistory } from '../api/watchHistory'
import HistoryPage from './HistoryPage'

vi.mock('../api/watchHistory', () => ({ listWatchHistory: vi.fn(), clearWatchHistory: vi.fn() }))
vi.mock('../contexts/AuthContext', () => ({
  useAuth: () => ({ user: { username: 'steven', role: 'viewer' }, isAdmin: false, logout: vi.fn() }),
}))
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn(), info: vi.fn() }))
vi.mock('../contexts/ToastContext', () => ({ useToast: () => toast }))

const item = (id: string, title: string) => ({
  id, video_id: `v-${id}`, title, duration_seconds: 600, progress_seconds: 60, completed: false, watched_at: new Date().toISOString(),
})

describe('HistoryPage', () => {
  beforeEach(() => vi.clearAllMocks())

  it('lands on an empty page 1 after clearing the history from a later page', async () => {
    let cleared = false
    vi.mocked(listWatchHistory).mockImplementation(async (page = 1) => ({
      data: cleared ? [] : [item(`h${page}`, `第${page}頁`)], total: cleared ? 0 : 21, page, page_size: 20,
    }) as never)
    vi.mocked(clearWatchHistory).mockImplementation(async () => { cleared = true })
    renderWithRouter(<HistoryPage />, { route: '/history' })
    await screen.findAllByText('第1頁')
    fireEvent.click(screen.getByText('2', { selector: 'button' }))
    await screen.findAllByText('第2頁')

    fireEvent.click(screen.getByText('清除全部', { selector: 'button' }))
    const dialog = screen.getByText('清除全部觀看記錄').closest('div')!
    fireEvent.click(within(dialog).getByText('清除全部', { selector: 'button' }))

    // It passes through loading page 1; assert where it settles.
    await waitFor(() => expect(listWatchHistory).toHaveBeenLastCalledWith(1, 20))
    await waitFor(() => expect(screen.getByText('還沒有觀看記錄')).toBeInTheDocument())
  })

  // Removing one row only hides it here (per the design): no request, total unchanged.
  it('hides a removed row without asking the server', async () => {
    vi.mocked(listWatchHistory).mockResolvedValue({ data: [item('h1', '片A'), item('h2', '片B')], total: 2, page: 1, page_size: 20 } as never)
    renderWithRouter(<HistoryPage />, { route: '/history' })
    await screen.findAllByText('片A')

    fireEvent.click(screen.getAllByLabelText('移除')[0])

    expect(screen.queryAllByText('片A')).toHaveLength(0)
    expect(screen.getAllByText('片B').length).toBeGreaterThan(0)
    expect(listWatchHistory).toHaveBeenCalledTimes(1)
  })
})
