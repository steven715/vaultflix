import { act, renderHook } from '@testing-library/react'
import type { ReactNode } from 'react'
import { MemoryRouter, useLocation, useNavigate } from 'react-router-dom'
import { describe, expect, it } from 'vitest'
import { useLibraryParams } from './useLibraryParams'

function params(route: string) {
  const wrapper = ({ children }: { children: ReactNode }) => <MemoryRouter initialEntries={[route]}>{children}</MemoryRouter>
  return renderHook(() => useLibraryParams(), { wrapper }).result
}

describe('useLibraryParams', () => {
  it('reads the backend defaults when the URL says nothing', () => {
    const p = params('/admin/library').current
    expect([p.page, p.pageSize, p.sortBy, p.sortOrder, p.query, p.tagIds]).toEqual([1, 20, 'created_at', 'desc', '', []])
  })

  it('reads the query from the URL', () => {
    const p = params('/?page=2&page_size=50&sort_by=title&sort_order=asc&q=abc&tag_ids=1,2').current
    expect([p.page, p.pageSize, p.sortBy, p.sortOrder, p.query, p.tagIds]).toEqual([2, 50, 'title', 'asc', 'abc', [1, 2]])
  })

  it('falls back to the defaults for sort values it does not know', () => {
    const p = params('/?sort_by=x&sort_order=up').current
    expect([p.sortBy, p.sortOrder]).toEqual(['created_at', 'desc'])
  })

  it('moves between pages', () => {
    const result = params('/?q=abc')
    act(() => result.current.setPage(3))
    expect(result.current.page).toBe(3)
    expect(result.current.query).toBe('abc')
  })

  // Each change of order or filter points into a different list.
  it.each([
    ['sorting by a column', (p: ReturnType<typeof useLibraryParams>) => p.sortByColumn('title')],
    ['changing the sort', (p: ReturnType<typeof useLibraryParams>) => p.setSort({ order: 'asc' })],
    ['filtering by Tags', (p: ReturnType<typeof useLibraryParams>) => p.setTagIds([4])],
  ])('goes back to page 1 when %s', (_, change) => {
    const result = params('/?page=3')
    act(() => change(result.current))
    expect(result.current.page).toBe(1)
  })

  it('flips the order of the current column, and starts another one descending', () => {
    const result = params('/?sort_by=title&sort_order=desc')
    act(() => result.current.sortByColumn('title'))
    expect([result.current.sortBy, result.current.sortOrder]).toEqual(['title', 'asc'])

    act(() => result.current.sortByColumn('duration_seconds'))
    expect([result.current.sortBy, result.current.sortOrder]).toEqual(['duration_seconds', 'desc'])
  })

  it('keeps the order when only the sort column changes', () => {
    const result = params('/?sort_order=asc')
    act(() => result.current.setSort({ by: 'title' }))
    expect([result.current.sortBy, result.current.sortOrder]).toEqual(['title', 'asc'])
  })
  // A correction replaces the URL: Back goes to where the admin came from,
  // not to the page that was corrected away.
  it('corrects the page without a history entry when asked to replace', () => {
    const wrapper = ({ children }: { children: ReactNode }) => (
      <MemoryRouter initialEntries={['/admin', '/admin/library?page=9']} initialIndex={1}>{children}</MemoryRouter>
    )
    const { result } = renderHook(() => ({ params: useLibraryParams(), navigate: useNavigate(), location: useLocation() }), { wrapper })

    act(() => result.current.params.setPage(2, { replace: true }))
    expect(result.current.location.search).toBe('?page=2')
    act(() => { void result.current.navigate(-1) })

    expect(result.current.location.pathname).toBe('/admin')
  })
})
