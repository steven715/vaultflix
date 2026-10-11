import { useToast } from '../contexts/ToastContext'

// useJobCancel wraps a Job's cancel (from useJob) for a progress panel: a
// failed request becomes a toast instead of an unhandled rejection. A kind
// that cannot be cancelled stays undefined, so the panel shows no button.
export function useJobCancel(cancel: (() => Promise<void>) | undefined): (() => Promise<void>) | undefined {
  const toast = useToast()
  if (!cancel) return undefined
  return async () => {
    try {
      await cancel()
    } catch (err) {
      console.warn('failed to cancel job', err)
      toast.error('取消失敗')
    }
  }
}
