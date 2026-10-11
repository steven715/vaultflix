import { useCallback, useEffect, useRef, useState } from 'react'

export type ResourceStatus = 'loading' | 'ready' | 'error'

/** Data a page shows from the API, with its loading and error states. */
export interface RemoteResource<T> {
  status: ResourceStatus
  /** Set only while ready; undefined while loading or in error. */
  data: T | undefined
  /** The failed load's error while in error. */
  error: unknown
  /**
   * Fetch again quietly, e.g. after an edit: the current data stays shown and
   * is replaced when the new data arrives. Rejects when the fetch fails,
   * leaving the current data; the page says so (usually a toast).
   */
  reload(): Promise<void>
  /** Fetch again through loading, e.g. the retry button of an error. */
  retry(): void
  /** Edit the loaded data in place, e.g. after a delete; no-op unless ready. */
  mutate(update: (prev: T) => T): void
}

interface State<T> {
  key: unknown
  status: ResourceStatus
  data?: T
  error?: unknown
}

// useRemoteResource loads what a page shows with load(), and loads it again
// from scratch whenever key changes (a date, a search query). Only the latest
// fetch may land: a response overtaken by a newer key, retry or reload, or
// arriving after unmount, is dropped. The data is never kept across keys, so a
// page cannot show one key's data under another.
export function useRemoteResource<T>(load: () => Promise<T>, key?: unknown): RemoteResource<T> {
  const [state, setState] = useState<State<T>>({ key, status: 'loading' })
  const loadRef = useRef(load)
  useEffect(() => {
    loadRef.current = load
  }, [load])
  // Bumped by every fetch and by leaving a key; a result lands only if its
  // fetch is still the latest.
  const generationRef = useRef(0)
  const keyRef = useRef(key)

  const dropPending = useCallback(() => { generationRef.current++ }, [])
  const fetchLatest = useCallback((forKey: unknown) => {
    const generation = ++generationRef.current
    const isLatest = () => generation === generationRef.current
    return loadRef.current().then(
      (data) => { if (isLatest()) setState({ key: forKey, status: 'ready', data }) },
      (error: unknown) => {
        if (!isLatest()) return
        throw error
      },
    )
  }, [])

  useEffect(() => {
    keyRef.current = key
    fetchLatest(key).catch((error: unknown) => setState({ key, status: 'error', error }))
    return dropPending
  }, [key, fetchLatest, dropPending])

  const reload = useCallback(() => fetchLatest(keyRef.current), [fetchLatest])
  const retry = useCallback(() => {
    const forKey = keyRef.current
    setState({ key: forKey, status: 'loading' })
    fetchLatest(forKey).catch((error: unknown) => setState({ key: forKey, status: 'error', error }))
  }, [fetchLatest])
  const mutate = useCallback((update: (prev: T) => T) => {
    setState((s) => (s.status === 'ready' ? { ...s, data: update(s.data as T) } : s))
  }, [])

  // Until the new key's first fetch lands, the previous key's state is stale.
  const current: State<T> = Object.is(state.key, key) ? state : { key, status: 'loading' }
  return { status: current.status, data: current.data, error: current.error, reload, retry, mutate }
}
