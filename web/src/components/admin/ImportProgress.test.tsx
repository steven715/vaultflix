import { describe, it, expect, vi } from 'vitest'
import { render, waitFor } from '@testing-library/react'
import ImportProgress from './ImportProgress'

vi.mock('../../contexts/WebSocketContext', () => ({
  useWS: () => ({ lastMessage: null }),
}))

vi.mock('../../api/admin', () => ({
  getActiveImportJob: () => Promise.resolve(null),
}))

describe('ImportProgress', () => {
  it('renders progress panel and uses no gray/indigo classes', async () => {
    const { container } = render(<ImportProgress jobId="j1" />)

    await waitFor(() => {
      expect(container.querySelector('.bg-gray-800\\/50')).toBeNull()
    })

    expect(container.innerHTML).not.toMatch(/(?:bg|text)-(?:gray|indigo)-/)
    expect(container.textContent).toContain('匯入進度')
  })
})
