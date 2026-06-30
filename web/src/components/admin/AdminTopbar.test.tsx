import { describe, it, expect, vi } from 'vitest'
import { screen, render, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, useLocation } from 'react-router-dom'
import AdminTopbar from './AdminTopbar'

vi.mock('../../contexts/AuthContext', () => ({
  useAuth: () => ({ user: { username: 'steven', role: 'admin' }, logout: vi.fn() }),
}))

function LocationProbe() {
  const loc = useLocation()
  return <div data-testid="loc">{loc.pathname + loc.search}</div>
}

describe('AdminTopbar', () => {
  it('shows breadcrumb + current page title', () => {
    render(
      <MemoryRouter initialEntries={['/admin/users']}>
        <AdminTopbar />
      </MemoryRouter>,
    )
    expect(screen.getByText('管理後台 /')).toBeInTheDocument()
    expect(screen.getByText('帳號')).toBeInTheDocument()
  })

  it('renders a 回前台 link back to the user-facing site', () => {
    render(
      <MemoryRouter initialEntries={['/admin/library']}>
        <AdminTopbar />
      </MemoryRouter>,
    )
    const back = screen.getByRole('link', { name: '回前台' })
    expect(back).toHaveAttribute('href', '/')
  })

  it('navigates to library with q on search input', async () => {
    render(
      <MemoryRouter initialEntries={['/admin/users']}>
        <AdminTopbar />
        <LocationProbe />
      </MemoryRouter>,
    )
    await userEvent.type(screen.getByPlaceholderText('搜尋影片、檔名、標籤...'), 'matrix')
    await waitFor(() =>
      expect(screen.getByTestId('loc').textContent).toBe('/admin/library?q=matrix'),
    )
  })
})
