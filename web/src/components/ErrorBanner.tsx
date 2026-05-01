type Props = {
  message: string
  onRetry?: () => void
  onDismiss?: () => void
}

export default function ErrorBanner({ message, onRetry, onDismiss }: Props) {
  return (
    <div
      role="alert"
      className="bg-red-900/30 border border-red-800 rounded-lg p-3 mb-4 flex items-center justify-between gap-3"
    >
      <span className="text-sm text-red-400 flex-1">{message}</span>
      <div className="flex items-center gap-3 shrink-0">
        {onRetry && (
          <button
            onClick={onRetry}
            className="text-sm text-red-300 hover:text-red-200 underline underline-offset-2"
          >
            重試
          </button>
        )}
        {onDismiss && (
          <button
            onClick={onDismiss}
            aria-label="關閉錯誤訊息"
            className="text-red-400 hover:text-red-300 text-sm"
          >
            ✕
          </button>
        )}
      </div>
    </div>
  )
}
