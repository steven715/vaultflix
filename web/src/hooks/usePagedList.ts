import { useEffect } from 'react'
import type { PaginatedResponse } from '../types'
import { useRemoteResource, type ResourceStatus } from './useRemoteResource'

/** One loaded page of a list: its items and the list's total. */
export interface PageOf<T> {
  items: T[]
  total: number
}

/** A page of a list a page shows, as usePagedList keeps it. */
export interface PagedList<T> {
  status: ResourceStatus
  /** The current page's items; empty while loading or in error. */
  items: T[]
  total: number
  totalPages: number
  error: unknown
  /** Fetch the page again quietly (e.g. after a Job): the items stay, a failure rejects. */
  reload(): Promise<void>
  /** Fetch the page again through loading, e.g. the retry button of an error. */
  retry(): void
  /** Edit the loaded page in place, e.g. drop a removed item and lower the total. */
  mutate(update: (page: PageOf<T>) => PageOf<T>): void
}

export interface PagedListOptions<T> {
  load: (page: number) => Promise<PaginatedResponse<T>>
  page: number
  pageSize: number
  /** The page's own way to change page (the URL, or state); replace corrects without a history entry. */
  setPage: (page: number, opts?: { replace?: boolean }) => void
  /** What besides the page number decides the list, e.g. the query and sort; a change loads from scratch. */
  key?: string
}

// usePagedList loads one page of a list with useRemoteResource, keyed by the
// page and key, so a page never shows another page's items or total. When
// the page turns out past the end (a delete emptied the last page, a stale
// bookmark), it moves to the last page, judged only by the current page's own
// total, and replaces the URL so Back does not land on the empty page again.
export function usePagedList<T>({ load, page, pageSize, setPage, key = '' }: PagedListOptions<T>): PagedList<T> {
  const resource = useRemoteResource<PageOf<T>>(
    () => load(page).then((res) => ({ items: res.data, total: res.total })),
    `${key}|${pageSize}|${page}`,
  )
  const items = resource.data?.items ?? []
  const total = resource.data?.total ?? 0
  const totalPages = Math.ceil(total / pageSize)

  const lastPage = Math.max(totalPages, 1)
  const ready = resource.status === 'ready'
  useEffect(() => {
    if (ready && page > lastPage) setPage(lastPage, { replace: true })
  }, [ready, page, lastPage, setPage])

  return {
    status: resource.status, items, total, totalPages, error: resource.error,
    reload: resource.reload, retry: resource.retry, mutate: resource.mutate,
  }
}
