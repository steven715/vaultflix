import { describe, it, expect } from 'vitest'
import { ADMIN_NAV, isNavItemActive, adminPageTitle } from './adminNav'

describe('ADMIN_NAV', () => {
  it('has 6 items in design order, all enabled', () => {
    expect(ADMIN_NAV.map((n) => n.key)).toEqual([
      'library', 'recommendations', 'tags', 'media-sources', 'users', 'analytics',
    ])
    expect(ADMIN_NAV.find((n) => n.key === 'dashboard')).toBeUndefined()
    expect(ADMIN_NAV.find((n) => n.key === 'analytics')!.enabled).toBe(true)
    expect(ADMIN_NAV.find((n) => n.key === 'library')!.enabled).toBe(true)
  })
})

describe('isNavItemActive', () => {
  it('treats /admin as the library route', () => {
    expect(isNavItemActive('/admin/library', '/admin')).toBe(true)
    expect(isNavItemActive('/admin/library', '/admin/library')).toBe(true)
  })
  it('matches exact path and sub-paths', () => {
    expect(isNavItemActive('/admin/users', '/admin/users')).toBe(true)
    expect(isNavItemActive('/admin/media-sources', '/admin/media-sources')).toBe(true)
    expect(isNavItemActive('/admin/users', '/admin/tags')).toBe(false)
  })
})

describe('adminPageTitle', () => {
  it('returns library label for /admin and /admin/library', () => {
    expect(adminPageTitle('/admin')).toBe('影片')
    expect(adminPageTitle('/admin/library')).toBe('影片')
  })
  it('returns the matching item label', () => {
    expect(adminPageTitle('/admin/tags')).toBe('標籤')
    expect(adminPageTitle('/admin/users')).toBe('帳號')
  })
})
