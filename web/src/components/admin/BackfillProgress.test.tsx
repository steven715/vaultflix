import { describe, it, expect, vi } from 'vitest'
import { render, waitFor } from '@testing-library/react'
import BackfillProgress from './BackfillProgress'

vi.mock('../../contexts/WebSocketContext', () => ({
  useWS: () => ({ lastMessage: null }),
}))

vi.mock('../../api/admin', () => ({
  getActiveBackfill: () => Promise.resolve(null),
  cancelBackfill: vi.fn(),
}))

vi.mock('../../contexts/ToastContext', () => ({
  useToast: () => ({ success: vi.fn(), error: vi.fn(), info: vi.fn() }),
}))

describe('BackfillProgress', () => {
  it('renders progress panel and uses no gray/indigo classes', async () => {
    const { container } = render(<BackfillProgress jobId="j1" />)

    await waitFor(() => {
      expect(container.textContent).toContain('補齊預覽中')
    })

    expect(container.innerHTML).not.toMatch(/(?:bg|text)-(?:gray|indigo)-/)
  })
})
