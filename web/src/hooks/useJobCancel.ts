import { useCallback } from 'react'
import { useToast } from '../contexts/ToastContext'

// useJobCancel wraps a Job's cancel (from useJob) for a progress panel: a
// failed request becomes a toast instead of an unhandled rejection. A kind
// that cannot be cancelled stays undefined, so the panel shows no button. The
// wrapper keeps its identity as long as cancel does.
export function useJobCancel(cancel: (() => Promise<void>) | undefined): (() => Promise<void>) | undefined {
  const toast = useToast()
  const wrapped = useCallback(async () => {
    try {
      await cancel?.()
    } catch (err) {
      console.warn('failed to cancel job', err)
      toast.error('取消失敗')
    }
  }, [cancel, toast])
  return cancel ? wrapped : undefined
}
