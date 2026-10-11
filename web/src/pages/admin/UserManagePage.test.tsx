import { describe, it, expect, vi, beforeEach } from 'vitest'
import { fireEvent, screen, waitFor } from '@testing-library/react'
import { renderWithRouter } from '../../test/renderWithRouter'
import UserManagePage from './UserManagePage'
import { listUsers } from '../../api/admin'

vi.mock('../../api/admin', () => ({
  listUsers: vi.fn(() => Promise.resolve([
    { id: 'u1', username: 'steven', role: 'admin', disabled_at: null, created_at: '2026-01-01T00:00:00Z', updated_at: '' },
    { id: 'u2', username: 'guest', role: 'viewer', disabled_at: null, created_at: '2026-01-02T00:00:00Z', updated_at: '' },
  ])),
  createUser: vi.fn(), deleteUser: vi.fn(), enableUser: vi.fn(), resetUserPassword: vi.fn(),
}))
vi.mock('../../contexts/ToastContext', () => ({ useToast: () => ({ success: vi.fn(), error: vi.fn(), info: vi.fn() }) }))

describe('UserManagePage (reskin)', () => {
  beforeEach(() => vi.clearAllMocks())
  it('renders the user table with role + status using tokens (no inline-style root)', async () => {
    const { container } = renderWithRouter(<UserManagePage />, { route: '/admin/users' })
    await waitFor(() => expect(screen.getByText('steven')).toBeInTheDocument())
    expect(screen.getByText('guest')).toBeInTheDocument()
    // 根容器不再用 inline style 的黑底
    const root = container.firstElementChild as HTMLElement
    expect(root.getAttribute('style')).toBeFalsy()
  })
  it('says the list failed to load instead of showing no users, and retries', async () => {
    vi.mocked(listUsers).mockRejectedValueOnce(new Error('down'))
    renderWithRouter(<UserManagePage />, { route: '/admin/users' })

    expect(await screen.findByRole('alert')).toHaveTextContent('無法載入使用者')
    expect(screen.queryByRole('table')).toBeNull()

    fireEvent.click(screen.getByText('重試'))
    expect(await screen.findByText('steven')).toBeInTheDocument()
  })
})
