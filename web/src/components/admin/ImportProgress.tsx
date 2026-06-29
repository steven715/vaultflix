import { useState, useEffect, useRef, useCallback } from 'react'
import { useWS } from '../../contexts/WebSocketContext'
import { getActiveImportJob } from '../../api/admin'
import type { WSMessage } from '../../hooks/useWebSocket'
import type { ImportJob, ImportProgress as ImportProgressType, ImportError } from '../../types'

type ImportState = 'importing' | 'completed' | 'failed'

interface ImportProgressProps {
  jobId: string
  onComplete?: () => void
}

export default function ImportProgress({ jobId, onComplete }: ImportProgressProps) {
  const onCompleteRef = useRef(onComplete)
  // Keep the ref pointing at the latest callback without writing during render.
  useEffect(() => {
    onCompleteRef.current = onComplete
  })
  const [importState, setImportState] = useState<ImportState>('importing')
  const [currentFile, setCurrentFile] = useState('')
  const [processed, setProcessed] = useState(0)
  const [importTotal, setImportTotal] = useState(0)
  const [imported, setImported] = useState(0)
  const [skipped, setSkipped] = useState(0)
  const [failed, setFailed] = useState(0)
  const [importErrors, setImportErrors] = useState<ImportError[]>([])
  const [finalResult, setFinalResult] = useState<ImportJob | null>(null)
  const [showErrors, setShowErrors] = useState(false)

  const { lastMessage } = useWS()

  // Restore progress from active job on mount
  useEffect(() => {
    let cancelled = false
    getActiveImportJob().then((job) => {
      if (cancelled || !job || job.id !== jobId) return
      setProcessed(job.processed)
      setImportTotal(job.total)
      setImported(job.imported)
      setSkipped(job.skipped)
      setFailed(job.failed)
      setImportErrors(job.errors || [])
    }).catch((err) => {
      console.warn('failed to restore import progress', err)
    })
    return () => { cancelled = true }
  }, [jobId])

  // Apply a single incoming WebSocket message to local progress state.
  const handleMessage = useCallback((msg: WSMessage) => {
    switch (msg.type) {
      case 'import_progress': {
        const p = msg.payload as ImportProgressType
        if (p.job_id !== jobId) break
        if (p.status === 'processing') {
          setCurrentFile(p.file_name)
        } else {
          setProcessed(p.current)
          setImportTotal(p.total)
          if (p.status === 'success') setImported((prev) => prev + 1)
          if (p.status === 'skipped') setSkipped((prev) => prev + 1)
          if (p.status === 'error') {
            setFailed((prev) => prev + 1)
            setImportErrors((prev) => [...prev, { file_name: p.file_name, error: p.error || '' }])
          }
        }
        break
      }
      case 'import_complete': {
        const result = msg.payload as ImportJob
        if (result.id !== jobId) break
        setFinalResult(result)
        setImportState(result.failed > 0 && result.imported === 0 ? 'failed' : 'completed')
        onCompleteRef.current?.()
        break
      }
      case 'import_error': {
        setImportState('failed')
        break
      }
    }
  }, [jobId])

  // WebSocket progress listener. Applying the update asynchronously keeps the
  // setState out of the synchronous effect body (accumulating counters use
  // functional updates, so deferring by a microtask preserves correctness).
  useEffect(() => {
    if (!lastMessage) return
    let cancelled = false
    const apply = async () => {
      if (!cancelled) handleMessage(lastMessage)
    }
    apply()
    return () => { cancelled = true }
  }, [lastMessage, handleMessage])

  return (
    <div className="bg-surface-2/50 rounded-lg p-4 mt-3">
      {importState === 'importing' && (
        <>
          <div className="mb-3">
            <div className="flex justify-between text-sm text-muted mb-1">
              <span>匯入進度</span>
              <span className="font-mono">{processed} / {importTotal || '...'}</span>
            </div>
            <div className="w-full bg-surface-up rounded-full h-2">
              <div
                className="bg-accent h-2 rounded-full transition-all duration-300"
                style={{ width: importTotal > 0 ? `${(processed / importTotal) * 100}%` : '0%' }}
              />
            </div>
          </div>
          {currentFile && (
            <p className="text-xs text-faint mb-2 truncate">處理中: {currentFile}</p>
          )}
          <div className="grid grid-cols-3 gap-2 text-sm">
            <div className="text-center">
              <div className="text-live font-medium font-mono">{imported}</div>
              <div className="text-faint text-xs">成功</div>
            </div>
            <div className="text-center">
              <div className="text-muted font-medium font-mono">{skipped}</div>
              <div className="text-faint text-xs">跳過</div>
            </div>
            <div className="text-center">
              <div className="text-fav font-medium font-mono">{failed}</div>
              <div className="text-faint text-xs">失敗</div>
            </div>
          </div>
        </>
      )}

      {(importState === 'completed' || importState === 'failed') && (
        <>
          <div className={`text-sm mb-3 font-medium ${importState === 'failed' ? 'text-fav' : 'text-live'}`}>
            {importState === 'completed' ? '匯入完成' : '匯入失敗'}
          </div>
          <div className="space-y-1.5 text-sm mb-3">
            <div className="flex justify-between text-cream"><span>掃描檔案</span><span className="font-mono">{finalResult?.total ?? importTotal}</span></div>
            <div className="flex justify-between text-live"><span>成功匯入</span><span className="font-mono">{finalResult?.imported ?? imported}</span></div>
            <div className="flex justify-between text-muted"><span>已跳過（重複）</span><span className="font-mono">{finalResult?.skipped ?? skipped}</span></div>
            <div className="flex justify-between text-fav"><span>失敗</span><span className="font-mono">{finalResult?.failed ?? failed}</span></div>
          </div>
          {(finalResult?.errors?.length ?? importErrors.length) > 0 && (
            <div>
              <button
                onClick={() => setShowErrors(!showErrors)}
                className="text-xs text-fav hover:text-fav/80 mb-1"
              >
                {showErrors ? '收起' : '展開'}失敗詳情 ({finalResult?.errors?.length ?? importErrors.length})
              </button>
              {showErrors && (
                <div className="bg-surface border border-border rounded p-2 max-h-40 overflow-y-auto space-y-1">
                  {(finalResult?.errors ?? importErrors).map((e, i) => (
                    <div key={i} className="text-xs">
                      <span className="text-cream">{e.file_name}</span>
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
