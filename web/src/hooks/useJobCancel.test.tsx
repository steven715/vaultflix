import { renderHook } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { useJobCancel } from './useJobCancel'

const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn(), info: vi.fn() }))
vi.mock('../contexts/ToastContext', () => ({ useToast: () => toast }))

describe('useJobCancel', () => {
  it('passes a kind that cannot be cancelled through as undefined', () => {
    const { result } = renderHook(() => useJobCancel(undefined))
    expect(result.current).toBeUndefined()
  })

  it('cancels, and toasts when the request fails', async () => {
    vi.spyOn(console, 'warn').mockImplementation(() => {})
    const ok = vi.fn(async () => {})
    const failing = vi.fn(async () => { throw new Error('500') })

    await renderHook(() => useJobCancel(ok)).result.current?.()
    expect(ok).toHaveBeenCalled()
    expect(toast.error).not.toHaveBeenCalled()

    await renderHook(() => useJobCancel(failing)).result.current?.()
    expect(toast.error).toHaveBeenCalledWith('取消失敗')
  })
})
