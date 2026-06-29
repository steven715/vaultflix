import { useEffect, useRef, useState } from 'react'
import { useWS } from '../../contexts/WebSocketContext'
import { cancelBackfill, getActiveBackfill } from '../../api/admin'
import { useToast } from '../../contexts/ToastContext'
import type {
  BackfillError,
  BackfillJob,
  BackfillProgress as BackfillProgressType,
} from '../../types'

type RunState = 'running' | 'completed' | 'failed' | 'cancelled'

interface BackfillProgressProps {
  jobId: string
  onComplete?: () => void
}

export default function BackfillProgress({ jobId, onComplete }: BackfillProgressProps) {
  const onCompleteRef = useRef(onComplete)
  onCompleteRef.current = onComplete

  const [runState, setRunState] = useState<RunState>('running')
  const [currentFile, setCurrentFile] = useState('')
  const [processed, setProcessed] = useState(0)
  const [total, setTotal] = useState(0)
  const [succeeded, setSucceeded] = useState(0)
  const [failed, setFailed] = useState(0)
  const [errors, setErrors] = useState<BackfillError[]>([])
  const [finalResult, setFinalResult] = useState<BackfillJob | null>(null)
  const [showErrors, setShowErrors] = useState(false)
  const [cancelling, setCancelling] = useState(false)

  const { lastMessage } = useWS()
  const toast = useToast()

  // Restore progress from active job on mount.
  useEffect(() => {
    let cancelled = false
    getActiveBackfill().then((job) => {
      if (cancelled || !job || job.id !== jobId) return
      setProcessed(job.processed)
      setTotal(job.total)
      setSucceeded(job.succeeded)
      setFailed(job.failed)
      setErrors(job.errors || [])
      if (job.status !== 'running') {
        setRunState(job.status)
        setFinalResult(job)
      }
    }).catch((err) => {
      console.warn('failed to restore backfill progress', err)
    })
    return () => { cancelled = true }
  }, [jobId])

  // WebSocket progress listener.
  useEffect(() => {
    if (!lastMessage) return

    switch (lastMessage.type) {
      case 'backfill_progress': {
        const p = lastMessage.payload as BackfillProgressType
        if (p.job_id !== jobId) break
        if (p.status === 'processing') {
          setCurrentFile(p.original_filename)
        } else {
          setProcessed(p.current)
          setTotal(p.total)
          if (p.status === 'success') setSucceeded((prev) => prev + 1)
          if (p.status === 'error') {
            setFailed((prev) => prev + 1)
            setErrors((prev) => [...prev, {
              video_id: p.video_id,
              original_filename: p.original_filename,
              error: p.error || '',
            }])
          }
        }
        break
      }
      case 'backfill_complete': {
        const result = lastMessage.payload as BackfillJob
        if (result.id !== jobId) break
        setFinalResult(result)
        setRunState(result.status as RunState)
        onCompleteRef.current?.()
        break
      }
      case 'backfill_error': {
        setRunState('failed')
        break
      }
    }
  }, [lastMessage, jobId])

  async function handleCancel() {
    if (cancelling) return
    setCancelling(true)
    try {
      await cancelBackfill(jobId)
    } catch (err) {
      console.warn('failed to cancel backfill', err)
      toast.error('取消失敗')
    } finally {
      setCancelling(false)
    }
  }

  const stateLabel: Record<RunState, string> = {
    running: '補齊中',
    completed: '補齊完成',
    failed: '補齊失敗',
    cancelled: '已取消',
  }
  const stateClass: Record<RunState, string> = {
    running: 'text-accent',
    completed: 'text-live',
    failed: 'text-fav',
    cancelled: 'text-faint',
  }
  const finalErrors = finalResult?.errors ?? errors

  return (
    <div className="bg-surface-2/50 rounded-lg p-4 mt-3">
      {runState === 'running' && (
        <>
          <div className="flex justify-between items-start mb-3">
            <div className="text-sm text-accent font-medium">補齊預覽中</div>
            <button
              onClick={handleCancel}
              disabled={cancelling}
              className="text-xs px-2 py-1 rounded-btn bg-fav/15 text-fav hover:bg-fav/25 disabled:opacity-50"
            >
              {cancelling ? '取消中...' : '取消'}
            </button>
          </div>
          <div className="mb-3">
            <div className="flex justify-between text-sm text-muted mb-1">
              <span>進度</span>
              <span className="font-mono">{processed} / {total || '...'}</span>
            </div>
            <div className="w-full bg-surface-up rounded-full h-2">
              <div
                className="bg-accent h-2 rounded-full transition-all duration-300"
                style={{ width: total > 0 ? `${(processed / total) * 100}%` : '0%' }}
              />
            </div>
          </div>
          {currentFile && (
            <p className="text-xs text-faint mb-2 truncate">處理中: {currentFile}</p>
          )}
          <div className="grid grid-cols-2 gap-2 text-sm">
            <div className="text-center">
              <div className="text-live font-medium font-mono">{succeeded}</div>
              <div className="text-faint text-xs">成功</div>
            </div>
            <div className="text-center">
              <div className="text-fav font-medium font-mono">{failed}</div>
              <div className="text-faint text-xs">失敗</div>
            </div>
          </div>
        </>
      )}

      {runState !== 'running' && (
        <>
          <div className={`text-sm mb-3 font-medium ${stateClass[runState]}`}>
            {stateLabel[runState]}
          </div>
          <div className="space-y-1.5 text-sm mb-3">
            <div className="flex justify-between text-cream"><span>掃描影片</span><span className="font-mono">{finalResult?.total ?? total}</span></div>
            <div className="flex justify-between text-cream"><span>已處理</span><span className="font-mono">{finalResult?.processed ?? processed}</span></div>
            <div className="flex justify-between text-live"><span>成功補齊</span><span className="font-mono">{finalResult?.succeeded ?? succeeded}</span></div>
            <div className="flex justify-between text-fav"><span>失敗</span><span className="font-mono">{finalResult?.failed ?? failed}</span></div>
          </div>
          {finalErrors.length > 0 && (
            <div>
              <button
                onClick={() => setShowErrors(!showErrors)}
                className="text-xs text-fav hover:text-fav/80 mb-1"
              >
                {showErrors ? '收起' : '展開'}失敗詳情 ({finalErrors.length})
              </button>
              {showErrors && (
                <div className="bg-surface border border-border rounded p-2 max-h-40 overflow-y-auto space-y-1">
                  {finalErrors.map((e, i) => (
                    <div key={i} className="text-xs">
                      <span className="text-cream">{e.original_filename}</span>
                      <span className="text-faint ml-1">— {e.error}</span>
                    </div>
                  ))}
                </div>
              )}
            </div>
          )}
        </>
      )}
    </div>
  )
}
