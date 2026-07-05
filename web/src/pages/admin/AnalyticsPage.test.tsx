import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, fireEvent } from '@testing-library/react'
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

  it('renders the trend when only views exist (hours round to 0) and toggles the metric', async () => {
    const viewsOnly: api.AnalyticsSummary = {
      ...empty,
      total_views: 5,
      daily_trend: empty.daily_trend.map((p, i) => (i === 29 ? { ...p, views: 5, watch_hours: 0 } : p)),
    }
    vi.spyOn(api, 'getAnalytics').mockResolvedValue(viewsOnly)
    render(<MemoryRouter><AnalyticsPage /></MemoryRouter>)
    // Default metric is watch_hours, but the chart must NOT show the empty state
    // because there are real views (hours just round to 0).
    await waitFor(() => expect(screen.getAllByText('近 30 天觀看時長').length).toBeGreaterThan(0))
    expect(screen.queryByText(/尚無觀看資料/)).toBeNull()
    // Toggling to 觀看次數 relabels the trend.
    fireEvent.click(screen.getByRole('button', { name: '觀看次數' }))
    expect(screen.getAllByText('近 30 天觀看次數').length).toBeGreaterThan(0)
  })
})
