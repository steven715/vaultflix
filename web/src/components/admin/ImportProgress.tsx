import { useState, useEffect } from 'react'
import { useWS } from '../../contexts/WebSocketContext'
import { getActiveImportJob } from '../../api/admin'
import type { ImportJob, ImportProgress as ImportProgressType, ImportError } from '../../types'

type ImportState = 'importing' | 'completed' | 'failed'

interface ImportProgressProps {
  jobId: string
  onComplete?: () => void
}

export default function ImportProgress({ jobId, onComplete }: ImportProgressProps) {
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
    }).catch(() => {})
    return () => { cancelled = true }
  }, [jobId])

  // WebSocket progress listener
  useEffect(() => {
    if (!lastMessage) return

    switch (lastMessage.type) {
      case 'import_progress': {
        const p = lastMessage.payload as ImportProgressType
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
        const result = lastMessage.payload as ImportJob
        if (result.id !== jobId) break
        setFinalResult(result)
        setImportState(result.failed > 0 && result.imported === 0 ? 'failed' : 'completed')
        onComplete?.()
        break
      }
      case 'import_error': {
        setImportState('failed')
        break
      }
    }
  }, [lastMessage, jobId, onComplete])

  return (
    <div className="bg-gray-800/50 rounded-lg p-4 mt-3">
      {importState === 'importing' && (
        <>
          <div className="mb-3">
            <div className="flex justify-between text-sm text-gray-400 mb-1">
              <span>匯入進度</span>
              <span>{processed} / {importTotal || '...'}</span>
            </div>
            <div className="w-full bg-gray-700 rounded-full h-2">
              <div
                className="bg-indigo-500 h-2 rounded-full transition-all duration-300"
                style={{ width: importTotal > 0 ? `${(processed / importTotal) * 100}%` : '0%' }}
              />
            </div>
          </div>
          {currentFile && (
            <p className="text-xs text-gray-500 mb-2 truncate">處理中: {currentFile}</p>
          )}
          <div className="grid grid-cols-3 gap-2 text-sm">
            <div className="text-center">
              <div className="text-green-400 font-medium">{imported}</div>
              <div className="text-gray-500 text-xs">成功</div>
            </div>
            <div className="text-center">
              <div className="text-gray-400 font-medium">{skipped}</div>
              <div className="text-gray-500 text-xs">跳過</div>
            </div>
            <div className="text-center">
              <div className="text-red-400 font-medium">{failed}</div>
              <div className="text-gray-500 text-xs">失敗</div>
            </div>
          </div>
        </>
      )}

      {(importState === 'completed' || importState === 'failed') && (
        <>
          <div className={`text-sm mb-3 font-medium ${importState === 'failed' ? 'text-red-400' : 'text-green-400'}`}>
            {importState === 'completed' ? '匯入完成' : '匯入失敗'}
          </div>
          <div className="space-y-1.5 text-sm mb-3">
            <div className="flex justify-between text-gray-300"><span>掃描檔案</span><span>{finalResult?.total ?? importTotal}</span></div>
            <div className="flex justify-between text-green-400"><span>成功匯入</span><span>{finalResult?.imported ?? imported}</span></div>
            <div className="flex justify-between text-gray-400"><span>已跳過（重複）</span><span>{finalResult?.skipped ?? skipped}</span></div>
            <div className="flex justify-between text-red-400"><span>失敗</span><span>{finalResult?.failed ?? failed}</span></div>
          </div>
          {(finalResult?.errors?.length ?? importErrors.length) > 0 && (
            <div>
              <button
                onClick={() => setShowErrors(!showErrors)}
                className="text-xs text-red-400 hover:text-red-300 mb-1"
              >
                {showErrors ? '收起' : '展開'}失敗詳情 ({finalResult?.errors?.length ?? importErrors.length})
              </button>
              {showErrors && (
                <div className="bg-gray-900 rounded p-2 max-h-40 overflow-y-auto space-y-1">
                  {(finalResult?.errors ?? importErrors).map((e, i) => (
                    <div key={i} className="text-xs">
                      <span className="text-gray-300">{e.file_name}</span>
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
