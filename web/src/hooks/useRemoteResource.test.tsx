import { act, renderHook, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { useRemoteResource } from './useRemoteResource'

// deferred hands a test the resolve/reject of one pending load.
function deferred<T>() {
  let resolve!: (v: T) => void
  let reject!: (e: unknown) => void
  const promise = new Promise<T>((res, rej) => { resolve = res; reject = rej })
  return { promise, resolve, reject }
}

describe('useRemoteResource', () => {
  it('is loading until the first load resolves, then ready with its data', async () => {
    const d = deferred<string[]>()
    const { result } = renderHook(() => useRemoteResource(() => d.promise))
    expect(result.current.status).toBe('loading')

    await act(async () => d.resolve(['a']))

    expect(result.current.status).toBe('ready')
    expect(result.current.data).toEqual(['a'])
  })

  it('is in error, not ready with no data, when the load fails', async () => {
    const { result } = renderHook(() => useRemoteResource(() => Promise.reject(new Error('down'))))

    await waitFor(() => expect(result.current.status).toBe('error'))
    expect(result.current.data).toBeUndefined()
    expect(result.current.error).toEqual(new Error('down'))
  })

  it('retries after an error through loading', async () => {
    const load = vi.fn().mockRejectedValueOnce(new Error('down')).mockResolvedValueOnce(['a'])
    const { result } = renderHook(() => useRemoteResource(load))
    await waitFor(() => expect(result.current.status).toBe('error'))

    act(() => result.current.retry())
    expect(result.current.status).toBe('loading')

    await waitFor(() => expect(result.current.status).toBe('ready'))
    expect(result.current.data).toEqual(['a'])
  })

  it('reloads quietly: the old data stays shown until the new data arrives', async () => {
    const second = deferred<string[]>()
    const load = vi.fn().mockResolvedValueOnce(['a']).mockReturnValueOnce(second.promise)
    const { result } = renderHook(() => useRemoteResource(load))
    await waitFor(() => expect(result.current.status).toBe('ready'))

    let reloading!: Promise<void>
    act(() => { reloading = result.current.reload() })
    expect(result.current.status).toBe('ready')
    expect(result.current.data).toEqual(['a'])

    await act(async () => { second.resolve(['a', 'b']); await reloading })
    expect(result.current.data).toEqual(['a', 'b'])
  })

  it('keeps the old data and rejects when a reload fails', async () => {
    const load = vi.fn().mockResolvedValueOnce(['a']).mockRejectedValueOnce(new Error('down'))
    const { result } = renderHook(() => useRemoteResource(load))
    await waitFor(() => expect(result.current.status).toBe('ready'))

    await act(async () => { await expect(result.current.reload()).rejects.toThrow('down') })

    expect(result.current.status).toBe('ready')
    expect(result.current.data).toEqual(['a'])
  })

  // A reload can overtake the first load (e.g. an Import ends while the page
  // is still loading); if it then fails, the page must show the error, not
  // stay on loading for good.
  it('ends in error when a reload overtakes the first load and fails', async () => {
    const first = deferred<string[]>()
    const load = vi.fn().mockReturnValueOnce(first.promise).mockRejectedValueOnce(new Error('down'))
    const { result } = renderHook(() => useRemoteResource(load))

    await act(async () => { await expect(result.current.reload()).rejects.toThrow('down') })
    await act(async () => first.resolve(['a']))

    expect(result.current.status).toBe('error')
  })

  it('loads again from scratch when the key changes', async () => {
    const loads: Record<string, ReturnType<typeof deferred<string>>> = { d1: deferred(), d2: deferred() }
    const { result, rerender } = renderHook(({ day }) => useRemoteResource(() => loads[day].promise, day), {
      initialProps: { day: 'd1' },
    })
    await act(async () => loads.d1.resolve('day one'))

    rerender({ day: 'd2' })
    expect(result.current.status).toBe('loading')
    expect(result.current.data).toBeUndefined()

    await act(async () => loads.d2.resolve('day two'))
    expect(result.current.data).toBe('day two')
  })

  it('drops a response that a newer key has overtaken', async () => {
    const loads: Record<string, ReturnType<typeof deferred<string>>> = { d1: deferred(), d2: deferred() }
    const { result, rerender } = renderHook(({ day }) => useRemoteResource(() => loads[day].promise, day), {
      initialProps: { day: 'd1' },
    })

    rerender({ day: 'd2' })
    await act(async () => loads.d2.resolve('day two'))
    await act(async () => loads.d1.resolve('day one'))

    expect(result.current.data).toBe('day two')
  })

  // A reload started for the old key (e.g. after adding to day one) must not
  // overwrite the list once the admin has moved to day two.
  it('drops a reload that a key change has overtaken', async () => {
    const reloadD1 = deferred<string>()
    const d2 = deferred<string>()
    const load = vi.fn((day: string) => (day === 'd1' ? (load.mock.calls.length === 1 ? Promise.resolve('day one') : reloadD1.promise) : d2.promise))
    const { result, rerender } = renderHook(({ day }) => useRemoteResource(() => load(day), day), {
      initialProps: { day: 'd1' },
    })
    await waitFor(() => expect(result.current.status).toBe('ready'))

    let reloading!: Promise<void>
    act(() => { reloading = result.current.reload() })
    rerender({ day: 'd2' })
    await act(async () => d2.resolve('day two'))
    await act(async () => { reloadD1.resolve('day one, reloaded'); await reloading })

    expect(result.current.data).toBe('day two')
  })

  it('edits the loaded data in place', async () => {
    const { result } = renderHook(() => useRemoteResource(() => Promise.resolve(['a', 'b'])))
    await waitFor(() => expect(result.current.status).toBe('ready'))

    act(() => result.current.mutate((prev) => prev.filter((x) => x !== 'a')))

    expect(result.current.data).toEqual(['b'])
  })
})
