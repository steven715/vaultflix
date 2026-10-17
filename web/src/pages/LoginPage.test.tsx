import { describe, it, expect, vi, beforeEach } from 'vitest'
import { fireEvent, screen } from '@testing-library/react'
import { renderWithRouter } from '../test/renderWithRouter'
import { ApiError } from '../api/errors'
import LoginPage from './LoginPage'

const login = vi.hoisted(() => vi.fn())
vi.mock('../contexts/AuthContext', () => ({ useAuth: () => ({ login, isAuthenticated: false }) }))

function submit() {
  fireEvent.change(screen.getByPlaceholderText('輸入帳號'), { target: { value: 'steven' } })
  fireEvent.change(screen.getByPlaceholderText('輸入密碼'), { target: { value: 'wrong' } })
  fireEvent.submit(screen.getByPlaceholderText('輸入密碼').closest('form')!)
}

describe('LoginPage', () => {
  beforeEach(() => vi.clearAllMocks())

  // The api client rejects with an ApiError; the page says why in Chinese.
  it.each([
    ['wrong password', new ApiError(401, 'unauthorized', 'invalid username or password', null), '帳號或密碼錯誤'],
    ['disabled account', new ApiError(403, 'forbidden', 'account is disabled', null), '此帳號已停用'],
    ['no answer from the server', new ApiError(null, null, null, new Error('Network Error')), '伺服器連線失敗，請稍後再試'],
  ])('says why a login failed: %s', async (_, err, shown) => {
    login.mockRejectedValueOnce(err)
    renderWithRouter(<LoginPage />, { route: '/login' })

    submit()

    expect(await screen.findByText(shown)).toBeInTheDocument()
  })
})
