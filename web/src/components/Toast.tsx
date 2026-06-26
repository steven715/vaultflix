import type { Toast } from '../contexts/ToastContext'

type Props = {
  toasts: Toast[]
  onDismiss: (id: string) => void
}

const KIND_STYLES: Record<Toast['kind'], string> = {
  success: 'bg-green-900/80 border-green-700 text-green-100',
  error: 'bg-red-900/80 border-red-700 text-red-100',
  info: 'bg-gray-800/90 border-gray-700 text-gray-100',
}

export default function ToastContainer({ toasts, onDismiss }: Props) {
  if (toasts.length === 0) return null
  return (
    <div className="fixed bottom-4 right-4 z-50 flex flex-col gap-2 pointer-events-none">
      {toasts.map((t) => (
        <div
          key={t.id}
          role="status"
          aria-live={t.kind === 'error' ? 'assertive' : 'polite'}
          className={`pointer-events-auto min-w-[240px] max-w-sm border rounded-lg px-3 py-2 text-sm shadow-lg flex items-center justify-between gap-3 ${KIND_STYLES[t.kind]}`}
        >
          <span className="break-words">{t.message}</span>
          <div className="flex items-center gap-2 shrink-0">
            {t.action && (
              <button
                onClick={() => {
                  t.action!.onClick()
                  onDismiss(t.id)
                }}
                className="font-semibold underline underline-offset-2 hover:opacity-80"
              >
                {t.action.label}
              </button>
            )}
            <button
              onClick={() => onDismiss(t.id)}
              className="text-current opacity-60 hover:opacity-100"
              aria-label="關閉提示"
            >
              ✕
            </button>
          </div>
        </div>
      ))}
    </div>
  )
}
