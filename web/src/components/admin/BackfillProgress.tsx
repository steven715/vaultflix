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
    running: 'text-indigo-300',
    completed: 'text-green-400',
    failed: 'text-red-400',
    cancelled: 'text-gray-400',
  }
  const finalErrors = finalResult?.errors ?? errors

  return (
    <div className="bg-gray-800/50 rounded-lg p-4 mt-3">
      {runState === 'running' && (
        <>
          <div className="flex justify-between items-start mb-3">
            <div className="text-sm text-indigo-300 font-medium">補齊預覽中</div>
            <button
              onClick={handleCancel}
              disabled={cancelling}
              className="text-xs px-2 py-1 rounded bg-red-900/40 text-red-300 hover:bg-red-900/60 disabled:opacity-50"
            >
              {cancelling ? '取消中...' : '取消'}
            </button>
          </div>
          <div className="mb-3">
            <div className="flex justify-between text-sm text-gray-400 mb-1">
              <span>進度</span>
              <span>{processed} / {total || '...'}</span>
            </div>
            <div className="w-full bg-gray-700 rounded-full h-2">
              <div
                className="bg-indigo-500 h-2 rounded-full transition-all duration-300"
                style={{ width: total > 0 ? `${(processed / total) * 100}%` : '0%' }}
              />
            </div>
          </div>
          {currentFile && (
            <p className="text-xs text-gray-500 mb-2 truncate">處理中: {currentFile}</p>
          )}
          <div className="grid grid-cols-2 gap-2 text-sm">
            <div className="text-center">
              <div className="text-green-400 font-medium">{succeeded}</div>
              <div className="text-gray-500 text-xs">成功</div>
            </div>
            <div className="text-center">
              <div className="text-red-400 font-medium">{failed}</div>
              <div className="text-gray-500 text-xs">失敗</div>
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
            <div className="flex justify-between text-gray-300"><span>掃描影片</span><span>{finalResult?.total ?? total}</span></div>
            <div className="flex justify-between text-gray-300"><span>已處理</span><span>{finalResult?.processed ?? processed}</span></div>
            <div className="flex justify-between text-green-400"><span>成功補齊</span><span>{finalResult?.succeeded ?? succeeded}</span></div>
            <div className="flex justify-between text-red-400"><span>失敗</span><span>{finalResult?.failed ?? failed}</span></div>
          </div>
          {finalErrors.length > 0 && (
            <div>
              <button
                onClick={() => setShowErrors(!showErrors)}
                className="text-xs text-red-400 hover:text-red-300 mb-1"
              >
                {showErrors ? '收起' : '展開'}失敗詳情 ({finalErrors.length})
              </button>
              {showErrors && (
                <div className="bg-gray-900 rounded p-2 max-h-40 overflow-y-auto space-y-1">
                  {finalErrors.map((e, i) => (
                    <div key={i} className="text-xs">
                      <span className="text-gray-300">{e.original_filename}</span>
                      <span className="text-gray-600 ml-1">— {e.error}</span>
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
