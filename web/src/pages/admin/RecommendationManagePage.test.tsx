import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import { renderWithRouter } from '../../test/renderWithRouter'
import RecommendationManagePage from './RecommendationManagePage'

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
})
