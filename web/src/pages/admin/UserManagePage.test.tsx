import { describe, it, expect, vi, beforeEach } from 'vitest'
import { fireEvent, screen, waitFor } from '@testing-library/react'
import { renderWithRouter } from '../../test/renderWithRouter'
import UserManagePage from './UserManagePage'
import { createUser, deleteUser, listUsers } from '../../api/admin'

vi.mock('../../api/admin', () => ({
  listUsers: vi.fn(() => Promise.resolve([
    { id: 'u1', username: 'steven', role: 'admin', disabled_at: null, created_at: '2026-01-01T00:00:00Z', updated_at: '' },
    { id: 'u2', username: 'guest', role: 'viewer', disabled_at: null, created_at: '2026-01-02T00:00:00Z', updated_at: '' },
  ])),
  createUser: vi.fn(), deleteUser: vi.fn(), enableUser: vi.fn(), resetUserPassword: vi.fn(),
}))
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn(), info: vi.fn() }))
vi.mock('../../contexts/ToastContext', () => ({ useToast: () => toast }))

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
  // The server's words are English for API callers; the admin reads what
  // went wrong in Chinese, never axios's "Request failed with status code 409".
  it('says the username is taken when creating a duplicate user', async () => {
    vi.mocked(createUser).mockRejectedValueOnce(Object.assign(new Error('Request failed with status code 409'), {
      response: { status: 409, data: { error: 'username_taken', message: 'username already exists' } },
    }))
    const { container } = renderWithRouter(<UserManagePage />, { route: '/admin/users' })
    fireEvent.click(await screen.findByText('建立使用者', { selector: 'button' }))
    fireEvent.change(container.querySelector('input:not([type=password])')!, { target: { value: 'steven' } })
    fireEvent.change(container.querySelector('input[type=password]')!, { target: { value: 'pass123' } })
    fireEvent.click(screen.getByText('建立', { selector: 'button' }))

    await waitFor(() => expect(toast.error).toHaveBeenCalledWith('使用者名稱已存在'))
  })

  it('says an admin cannot be disabled', async () => {
    vi.mocked(deleteUser).mockRejectedValueOnce(Object.assign(new Error('Request failed with status code 400'), {
      response: { status: 400, data: { error: 'cannot_disable_admin', message: 'cannot disable admin account' } },
    }))
    renderWithRouter(<UserManagePage />, { route: '/admin/users' })
    fireEvent.click(await screen.findByText('停用', { selector: 'button' }))
    fireEvent.click(screen.getByText('確認停用', { selector: 'button' }))

    await waitFor(() => expect(toast.error).toHaveBeenCalledWith('不能停用管理員帳號'))
  })
})
