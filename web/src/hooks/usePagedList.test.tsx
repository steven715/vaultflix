import { act, renderHook, waitFor } from '@testing-library/react'
import { useState } from 'react'
import { describe, expect, it, vi } from 'vitest'
import type { PaginatedResponse } from '../types'
import { usePagedList } from './usePagedList'

const PAGE_SIZE = 20

function pageOf(page: number, total: number, ids: string[]): PaginatedResponse<string> {
  return { data: ids, total, page, page_size: PAGE_SIZE }
}

// A page that keeps its page number in state, as FavoritesPage does.
function render(load: (page: number) => Promise<PaginatedResponse<string>>, initialPage = 1, key = '') {
  return renderHook(({ k }) => {
    const [page, setPage] = useState(initialPage)
    return { page, setPage, list: usePagedList({ load, page, pageSize: PAGE_SIZE, setPage, key: k }) }
  }, { initialProps: { k: key } })
}

describe('usePagedList', () => {
  it('loads the page’s items and total', async () => {
    const { result } = render(async (page) => pageOf(page, 21, ['a', 'b']))

    await waitFor(() => expect(result.current.list.status).toBe('ready'))
    expect(result.current.list.items).toEqual(['a', 'b'])
    expect([result.current.list.total, result.current.list.totalPages]).toEqual([21, 2])
  })

  // A stale page number (a bookmark, a page emptied elsewhere) shows the last page.
  it('moves to the last page when the page is past the end', async () => {
    const load = vi.fn(async (page: number) => pageOf(page, 21, page === 2 ? ['u'] : []))
    const { result } = render(load, 5)

    await waitFor(() => expect(result.current.page).toBe(2))
    await waitFor(() => expect(result.current.list.items).toEqual(['u']))
  })

  it('goes to page 1 once nothing is left', async () => {
    const { result } = render(async (page) => pageOf(page, 0, []), 3)

    await waitFor(() => expect(result.current.page).toBe(1))
  })

  // Removing the only item of the last page leaves a page past the end.
  it('moves to the new last page when a local edit empties the current one', async () => {
    const load = vi.fn(async (page: number) => pageOf(page, page === 2 ? 21 : 20, page === 2 ? ['u'] : ['p1']))
    const { result } = render(load, 2)
    await waitFor(() => expect(result.current.list.items).toEqual(['u']))

    act(() => result.current.list.mutate((p) => ({ items: p.items.filter((x) => x !== 'u'), total: p.total - 1 })))

    await waitFor(() => expect(result.current.page).toBe(1))
    await waitFor(() => expect(result.current.list.items).toEqual(['p1']))
  })

  // Until a page's own total arrives, the previous page's total says nothing about it.
  it('keeps a later page while its total is still loading', async () => {
    let resolvePage3!: (r: PaginatedResponse<string>) => void
    const load = vi.fn((page: number) => (page === 3
      ? new Promise<PaginatedResponse<string>>((r) => { resolvePage3 = r })
      : Promise.resolve(pageOf(page, 5, ['a']))))
    const { result } = render(load)
    await waitFor(() => expect(result.current.list.status).toBe('ready'))

    act(() => result.current.setPage(3))
    expect(result.current.list.status).toBe('loading')
    await act(async () => resolvePage3(pageOf(3, 60, ['c'])))

    expect(result.current.page).toBe(3)
    expect(result.current.list.items).toEqual(['c'])
  })

  it('drops the items of the previous query when the key changes', async () => {
    const { result, rerender } = render(async (page) => pageOf(page, 1, ['a']), 1, 'q=a')
    await waitFor(() => expect(result.current.list.status).toBe('ready'))

    rerender({ k: 'q=b' })

    expect(result.current.list.status).toBe('loading')
    expect(result.current.list.items).toEqual([])
  })

  // A quiet reload (after a Job) keeps the rows; its failure is the page's to say.
  it('keeps the items when a reload fails', async () => {
    const load = vi.fn().mockResolvedValueOnce(pageOf(1, 1, ['a'])).mockRejectedValueOnce(new Error('down'))
    const { result } = render(load)
    await waitFor(() => expect(result.current.list.status).toBe('ready'))

    await act(async () => { await expect(result.current.list.reload()).rejects.toThrow('down') })

    expect(result.current.list.status).toBe('ready')
    expect(result.current.list.items).toEqual(['a'])
  })
  // The correction rewrites where the admin is, not where they went: Back must
  // not land on the page that was corrected away.
  it('corrects the page with replace', async () => {
    const setPage = vi.fn()
    renderHook(() => usePagedList({ load: async (page) => pageOf(page, 21, []), page: 5, pageSize: PAGE_SIZE, setPage }))

    await waitFor(() => expect(setPage).toHaveBeenCalledWith(2, { replace: true }))
  })

  it('retries a failed page through loading', async () => {
    const load = vi.fn().mockRejectedValueOnce(new Error('down')).mockResolvedValueOnce(pageOf(1, 1, ['a']))
    const { result } = render(load)
    await waitFor(() => expect(result.current.list.status).toBe('error'))

    act(() => result.current.list.retry())

    await waitFor(() => expect(result.current.list.items).toEqual(['a']))
  })
})
