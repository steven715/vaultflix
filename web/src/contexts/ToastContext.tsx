import { createContext, useContext, useState, useRef, useCallback, useEffect, useMemo } from 'react'
import type { ReactNode } from 'react'
import ToastContainer from '../components/Toast'

export type ToastKind = 'success' | 'error' | 'info'

export type Toast = {
  id: string
  kind: ToastKind
  message: string
}

const TOAST_TTL_MS = 3000
const MAX_STACK = 3

type ToastAPI = {
  success: (message: string) => void
  error: (message: string) => void
  info: (message: string) => void
}

const ToastContext = createContext<ToastAPI | null>(null)

export function useToast(): ToastAPI {
  const ctx = useContext(ToastContext)
  if (!ctx) throw new Error('useToast must be used within ToastProvider')
  return ctx
}

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

  const push = useCallback((kind: ToastKind, message: string) => {
    let scheduledId: string | null = null
    let evictedId: string | null = null

    setToasts((prev) => {
      const existing = prev.find((t) => t.kind === kind && t.message === message)
      if (existing) {
        scheduledId = existing.id
        return prev
      }
      const id = String(++nextIdRef.current)
      scheduledId = id
      const next = [...prev, { id, kind, message }]
      if (next.length > MAX_STACK) {
        const evicted = next.shift()!
        evictedId = evicted.id
      }
      return next
    })

    if (scheduledId) scheduleDismiss(scheduledId)
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
    success: (message) => push('success', message),
    error: (message) => push('error', message),
    info: (message) => push('info', message),
  }), [push])

  return (
    <ToastContext.Provider value={api}>
      {children}
      <ToastContainer toasts={toasts} onDismiss={dismiss} />
    </ToastContext.Provider>
  )
}
