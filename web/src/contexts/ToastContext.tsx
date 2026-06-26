import { createContext, useContext } from 'react'

export type ToastKind = 'success' | 'error' | 'info'

export type ToastAction = {
  label: string
  onClick: () => void
}

export type ToastOptions = {
  // 帶 onClick 的行動按鈕（如「有新版，點此更新」）
  action?: ToastAction
  // 不自動消失，需使用者主動關閉或點擊行動按鈕（搭配 action 使用）
  persist?: boolean
}

export type Toast = {
  id: string
  kind: ToastKind
  message: string
  action?: ToastAction
  persist?: boolean
}

export type ToastAPI = {
  success: (message: string, opts?: ToastOptions) => void
  error: (message: string, opts?: ToastOptions) => void
  info: (message: string, opts?: ToastOptions) => void
}

export const ToastContext = createContext<ToastAPI | null>(null)

export function useToast(): ToastAPI {
  const ctx = useContext(ToastContext)
  if (!ctx) throw new Error('useToast must be used within ToastProvider')
  return ctx
}
