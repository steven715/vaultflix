import { useState, useRef, useCallback, useEffect, useMemo } from 'react'
import type { ReactNode } from 'react'
import ToastContainer from '../components/Toast'
import { ToastContext } from './ToastContext'
import type { Toast, ToastKind, ToastAPI, ToastOptions } from './ToastContext'

const TOAST_TTL_MS = 3000
const MAX_STACK = 3

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([])
  const timeoutsRef = useRef<Map<string, ReturnType<typeof setTimeout>>>(new Map())
  const nextIdRef = useRef(0)

  const dismiss = useCallback((id: string) => {
    setToasts((prev) => prev.filter((t) => t.id !== id))
    const timeoutId = timeoutsRef.current.get(id)
    if (timeoutId) {
      clearTimeout(timeoutId)
      timeoutsRef.current.delete(id)
    }
  }, [])

  const scheduleDismiss = useCallback((id: string) => {
    const existing = timeoutsRef.current.get(id)
    if (existing) clearTimeout(existing)
    const timeoutId = setTimeout(() => dismiss(id), TOAST_TTL_MS)
    timeoutsRef.current.set(id, timeoutId)
  }, [dismiss])

  const push = useCallback((kind: ToastKind, message: string, opts?: ToastOptions) => {
    let scheduledId: string | null = null
    let evictedId: string | null = null
    const persist = opts?.persist ?? false

    setToasts((prev) => {
      const existing = prev.find((t) => t.kind === kind && t.message === message)
      if (existing) {
        scheduledId = existing.id
        return prev
      }
      const id = String(++nextIdRef.current)
      scheduledId = id
      const next = [...prev, { id, kind, message, action: opts?.action, persist }]
      if (next.length > MAX_STACK) {
        const evicted = next.shift()!
        evictedId = evicted.id
      }
      return next
    })

    // persist 的 toast 不排程自動消失，靠使用者關閉或點行動按鈕
    if (scheduledId && !persist) scheduleDismiss(scheduledId)
    if (evictedId) {
      const t = timeoutsRef.current.get(evictedId)
      if (t) {
        clearTimeout(t)
        timeoutsRef.current.delete(evictedId)
      }
    }
  }, [scheduleDismiss])

  useEffect(() => {
    const timeouts = timeoutsRef.current
    return () => {
      for (const id of timeouts.values()) clearTimeout(id)
      timeouts.clear()
    }
  }, [])

  const api = useMemo<ToastAPI>(() => ({
    success: (message, opts) => push('success', message, opts),
    error: (message, opts) => push('error', message, opts),
    info: (message, opts) => push('info', message, opts),
  }), [push])

  return (
    <ToastContext.Provider value={api}>
      {children}
      <ToastContainer toasts={toasts} onDismiss={dismiss} />
    </ToastContext.Provider>
  )
}
