import { useCallback, useMemo } from 'react'
import { useSearchParams } from 'react-router-dom'
import {
  parseSortBy, parseSortOrder, parseTagIds, serializeTagIds, toggleSort, type LibrarySortBy, type SortOrder,
} from '../lib/libraryParams'
import type { VideoListParams } from '../types'

/** The Library query a page shows, kept in the URL. */
export interface LibraryParams {
  page: number
  pageSize: number
  sortBy: LibrarySortBy
  sortOrder: SortOrder
  query: string
  tagIds: number[]
  /** tag_ids as in the URL: a stable effect dependency, unlike the array. */
  tagIdsParam: string
  /** The video list request for this query, without the page (listVideos takes it with one). */
  filter: Omit<VideoListParams, 'page'>
  /** replace: correct the URL without a history entry, so Back does not return to the corrected page. */
  setPage(page: number, opts?: { replace?: boolean }): void
  /** Sort by col: the current column flips its order, another starts newest/largest first. */
  sortByColumn(col: LibrarySortBy): void
  setSort(sort: { by?: LibrarySortBy; order?: SortOrder }): void
  setTagIds(ids: number[]): void
}

const defaultPageSize = 20

// useLibraryParams reads and writes the Library query in the URL with the
// backend's defaults (page 1, 20 per page, newest first); a garbled value
// falls back to its default. Any change of sort or filter goes back to page 1,
// because the old page number points into a different list.
export function useLibraryParams(): LibraryParams {
  const [searchParams, setSearchParams] = useSearchParams()

  const page = Number(searchParams.get('page')) || 1
  const pageSize = Number(searchParams.get('page_size')) || defaultPageSize
  const sortBy = parseSortBy(searchParams.get('sort_by')) ?? 'created_at'
  const sortOrder = parseSortOrder(searchParams.get('sort_order')) ?? 'desc'
  const query = searchParams.get('q') || ''
  const tagIdsParam = searchParams.get('tag_ids') || ''
  const tagIds = useMemo(() => parseTagIds(tagIdsParam), [tagIdsParam])
  const filter = { page_size: pageSize, sort_by: sortBy, sort_order: sortOrder, q: query || undefined, tag_ids: tagIdsParam || undefined }

  const update = useCallback((updates: Record<string, string>, opts?: { replace?: boolean }) => {
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev)
      for (const [key, value] of Object.entries(updates)) {
        if (value) next.set(key, value)
        else next.delete(key)
      }
      return next
    }, opts)
  }, [setSearchParams])

  const setPage = useCallback(
    (p: number, opts?: { replace?: boolean }) => update({ page: p > 1 ? String(p) : '' }, opts),
    [update],
  )
  const setSort = useCallback(
    ({ by, order }: { by?: LibrarySortBy; order?: SortOrder }) =>
      update({ ...(by && { sort_by: by }), ...(order && { sort_order: order }), page: '' }),
    [update],
  )
  const sortByColumn = useCallback(
    (col: LibrarySortBy) => {
      const next = toggleSort({ by: sortBy, order: sortOrder }, col)
      setSort({ by: next.by, order: next.order })
    },
    [sortBy, sortOrder, setSort],
  )
  const setTagIds = useCallback((ids: number[]) => update({ tag_ids: serializeTagIds(ids), page: '' }), [update])

  return { page, pageSize, sortBy, sortOrder, query, tagIds, tagIdsParam, filter, setPage, sortByColumn, setSort, setTagIds }
}
