import { describe, it, expect, vi, beforeEach } from 'vitest'
import { fireEvent, screen, waitFor } from '@testing-library/react'
import { renderWithRouter } from '../../test/renderWithRouter'
import TagManagePage from './TagManagePage'
import { listTags } from '../../api/tags'

vi.mock('../../api/tags', () => ({
  listTags: vi.fn(() => Promise.resolve([
    { id: 1, name: '動作', category: 'genre', video_count: 12 },
    { id: 2, name: 'A工作室', category: 'studio', video_count: 5 },
  ])),
}))
vi.mock('../../api/admin', () => ({ createTag: vi.fn() }))
vi.mock('../../contexts/ToastContext', () => ({
  useToast: () => ({ success: vi.fn(), error: vi.fn(), info: vi.fn() }),
}))

describe('TagManagePage', () => {
  beforeEach(() => vi.clearAllMocks())
  it('renders category groups with their tags', async () => {
    renderWithRouter(<TagManagePage />, { route: '/admin/tags' })
    await waitFor(() => expect(screen.getByText('類型')).toBeInTheDocument())
    expect(screen.getByText('工作室')).toBeInTheDocument()
    expect(screen.getByText('動作')).toBeInTheDocument()
    expect(screen.getByText('A工作室')).toBeInTheDocument()
  })
  it('shows loading until the tags arrive', async () => {
    renderWithRouter(<TagManagePage />, { route: '/admin/tags' })
    expect(screen.getByText('載入中...')).toBeInTheDocument()
    expect(await screen.findByText('動作')).toBeInTheDocument()
    expect(screen.queryByText('載入中...')).toBeNull()
  })

  it('says the tags failed to load, and retries', async () => {
    vi.mocked(listTags).mockRejectedValueOnce(new Error('down'))
    renderWithRouter(<TagManagePage />, { route: '/admin/tags' })

    expect(await screen.findByRole('alert')).toHaveTextContent('無法載入標籤')
    fireEvent.click(screen.getByText('重試'))
    expect(await screen.findByText('動作')).toBeInTheDocument()
  })
})
