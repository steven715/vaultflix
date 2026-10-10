import { describe, it, expect, vi } from 'vitest'
import { render, waitFor } from '@testing-library/react'
import BackfillProgress from './BackfillProgress'

vi.mock('../../contexts/WebSocketContext', () => ({
  useWS: () => ({ lastMessage: null }),
}))

vi.mock('../../api/admin', () => ({
  getActiveBackfill: vi.fn(() => Promise.resolve(null)),
  cancelBackfill: vi.fn(),
}))

vi.mock('../../contexts/ToastContext', () => ({
  useToast: () => ({ success: vi.fn(), error: vi.fn(), info: vi.fn() }),
}))

describe('BackfillProgress', () => {
  it('renders progress panel and uses no gray/indigo classes', async () => {
    const { container } = render(<BackfillProgress jobId="j1" kind="preview" />)

    await waitFor(() => {
      expect(container.textContent).toContain('補齊預覽中')
    })

    expect(container.innerHTML).not.toMatch(/(?:bg|text)-(?:gray|indigo)-/)
  })

  it('names the kind of Backfill that is running', async () => {
    const { container } = render(<BackfillProgress jobId="j1" kind="keyframe" />)

    await waitFor(() => {
      expect(container.textContent).toContain('補齊 Keyframe Index 中')
    })
  })

  it('counts skipped Videos restored from the active job', async () => {
    const { getActiveBackfill } = await import('../../api/admin')
    vi.mocked(getActiveBackfill).mockResolvedValueOnce({
      id: 'j1', kind: 'codec', status: 'completed', total: 3, processed: 3,
      succeeded: 1, failed: 0, skipped: 2, errors: [], started_at: '2026-10-10T00:00:00Z',
    })
    const { container } = render(<BackfillProgress jobId="j1" kind="codec" />)

    await waitFor(() => {
      expect(container.textContent).toMatch(/略過\s*2/)
    })
  })
})
