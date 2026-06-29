import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
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

    await screen.findByText('匯入進度')

    expect(container.innerHTML).not.toMatch(/(?:bg|text)-(?:gray|indigo)-/)
  })
})
