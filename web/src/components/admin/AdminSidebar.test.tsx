import { describe, it, expect } from 'vitest'
import { screen } from '@testing-library/react'
import { renderWithRouter } from '../../test/renderWithRouter'
import AdminSidebar from './AdminSidebar'

describe('AdminSidebar', () => {
  it('renders all 6 nav labels', () => {
    renderWithRouter(<AdminSidebar />, { route: '/admin/library' })
    for (const label of ['影片', '精選', '標籤', '來源', '帳號', '分析']) {
      expect(screen.getByText(label)).toBeInTheDocument()
    }
    expect(screen.queryByText('總覽')).toBeNull()
  })
  it('renders all items as links now that every nav item is enabled', () => {
    renderWithRouter(<AdminSidebar />, { route: '/admin/library' })
    // 影片 and 分析 are both enabled → their labels sit inside anchors
    expect(screen.getByText('影片').closest('a')).not.toBeNull()
    expect(screen.getByText('分析').closest('a')).not.toBeNull()
  })
  it('marks the active item with aria-current', () => {
    renderWithRouter(<AdminSidebar />, { route: '/admin/users' })
    expect(screen.getByText('帳號').closest('a')).toHaveAttribute('aria-current', 'page')
  })
})
