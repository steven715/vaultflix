import { describe, it, expect, vi, beforeEach } from 'vitest'
import { act, fireEvent, screen, waitFor } from '@testing-library/react'
import { renderWithRouter } from '../../test/renderWithRouter'
import RecommendationManagePage from './RecommendationManagePage'
import { listRecommendationsByDate, updateRecommendationSortOrder } from '../../api/admin'
import type { RecommendationItem } from '../../types'

vi.mock('../../api/admin', () => ({
  listRecommendationsByDate: vi.fn(() => Promise.resolve([
    { id: 'r1', video_id: 'v1', title: '片A', thumbnail_url: undefined, duration_seconds: 600, resolution: '1080p', file_size_bytes: 1, sort_order: 1, is_fallback: false },
  ])),
  createRecommendation: vi.fn(), updateRecommendationSortOrder: vi.fn(), deleteRecommendation: vi.fn(),
}))
vi.mock('../../contexts/ToastContext', () => ({ useToast: () => ({ success: vi.fn(), error: vi.fn(), info: vi.fn() }) }))

describe('RecommendationManagePage (reskin)', () => {
  beforeEach(() => vi.clearAllMocks())
  it('lists recommendations for the date with token styling', async () => {
    const { container } = renderWithRouter(<RecommendationManagePage />, { route: '/admin/recommendations' })
    await waitFor(() => expect(screen.getByText('片A')).toBeInTheDocument())
    expect(container.querySelector('.bg-gray-950')).toBeNull()
  })
  // Re-sorting reloads the day's list; if the admin moves to another day before
  // that reload returns, the reload must not overwrite the new day's list.
  it('keeps the new day when a reload for the previous day returns late', async () => {
    const rec = (id: string, title: string): RecommendationItem => ({
      id, video_id: 'v' + id, title, duration_seconds: 600, resolution: '1080p', file_size_bytes: 1, sort_order: 1, is_fallback: false,
    } as RecommendationItem)
    let resolveLateReload!: (items: RecommendationItem[]) => void
    vi.mocked(updateRecommendationSortOrder).mockResolvedValue(undefined as never)
    vi.mocked(listRecommendationsByDate)
      .mockResolvedValueOnce([rec('r1', '片A')])
      .mockImplementationOnce(() => new Promise((res) => { resolveLateReload = res }))
      .mockResolvedValueOnce([rec('r2', '明天的片')])
    renderWithRouter(<RecommendationManagePage />, { route: '/admin/recommendations' })
    const order = await screen.findByDisplayValue('1')

    fireEvent.change(order, { target: { value: '2' } })
    fireEvent.blur(order)
    await waitFor(() => expect(listRecommendationsByDate).toHaveBeenCalledTimes(2))
    fireEvent.change(screen.getByDisplayValue(/^\d{4}-\d{2}-\d{2}$/), { target: { value: '2030-01-02' } })
    expect(await screen.findByText('明天的片')).toBeInTheDocument()

    await act(async () => resolveLateReload([rec('r1', '片A')]))
    expect(screen.getByText('明天的片')).toBeInTheDocument()
    expect(screen.queryByText('片A')).toBeNull()
  })
})
