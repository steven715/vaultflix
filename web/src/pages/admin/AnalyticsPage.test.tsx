import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import AnalyticsPage from './AnalyticsPage'
import * as api from '../../api/analytics'

const empty: api.AnalyticsSummary = {
  range_days: 30,
  total_views: 0,
  total_watch_hours: 0,
  avg_completion_rate: 0,
  active_users: 0,
  daily_trend: Array.from({ length: 30 }, (_, i) => ({ date: `2026-06-${String(i + 1).padStart(2, '0')}`, views: 0, watch_hours: 0 })),
  top_videos: [],
  top_tags: [],
}

describe('AnalyticsPage', () => {
  beforeEach(() => vi.restoreAllMocks())

  it('renders KPI values from the API', async () => {
    vi.spyOn(api, 'getAnalytics').mockResolvedValue({ ...empty, total_views: 42, active_users: 3 })
    render(<MemoryRouter><AnalyticsPage /></MemoryRouter>)
    await waitFor(() => expect(screen.getByText('42')).toBeInTheDocument())
    expect(screen.getByText('3')).toBeInTheDocument()
  })

  it('shows the empty chart state when there is no watch data', async () => {
    vi.spyOn(api, 'getAnalytics').mockResolvedValue(empty)
    render(<MemoryRouter><AnalyticsPage /></MemoryRouter>)
    await waitFor(() => expect(screen.getAllByText(/尚無觀看資料/).length).toBeGreaterThan(0))
  })
})
