import { describe, it, expect, vi } from 'vitest'
import { screen, render } from '@testing-library/react'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import AdminLayout from './AdminLayout'

vi.mock('../../contexts/AuthContext', () => ({
  useAuth: () => ({ user: { username: 'steven', role: 'admin' }, logout: vi.fn() }),
}))

describe('AdminLayout', () => {
  it('renders sidebar + topbar around the outlet content', () => {
    render(
      <MemoryRouter initialEntries={['/admin/library']}>
        <Routes>
          <Route element={<AdminLayout />}>
            <Route path="/admin/library" element={<div>庫內容</div>} />
          </Route>
        </Routes>
      </MemoryRouter>,
    )
    expect(screen.getByText('庫內容')).toBeInTheDocument()
    expect(screen.getAllByText('影片')).toHaveLength(2) // sidebar 標籤 + topbar 頁面標題
    expect(screen.getByText('管理後台 /')).toBeInTheDocument() // topbar 麵包屑
  })
})
