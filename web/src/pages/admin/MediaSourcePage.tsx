import { useState, useEffect } from 'react'
import { listMediaSources, createMediaSource, updateMediaSource, deleteMediaSource, importVideos, getActiveImportJob } from '../../api/admin'
import type { MediaSource } from '../../types'
import Header from '../../components/Header'
import ImportProgress from '../../components/admin/ImportProgress'

export default function MediaSourcePage() {
  const [sources, setSources] = useState<MediaSource[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [loadFailed, setLoadFailed] = useState(false)

  // Import state
  const [importingSourceId, setImportingSourceId] = useState<string | null>(null)
  const [currentJobId, setCurrentJobId] = useState<string | null>(null)

  // Modal state
  const [showCreate, setShowCreate] = useState(false)
  const [editingSource, setEditingSource] = useState<MediaSource | null>(null)
  const [confirmAction, setConfirmAction] = useState<{ type: 'disable' | 'enable' | 'delete'; source: MediaSource } | null>(null)

  async function fetchSources() {
    setLoading(true)
    setError(null)
    setLoadFailed(false)
    try {
      const data = await listMediaSources()
      setSources(data)
    } catch {
      setError('無法載入媒體來源，請確認服務是否正常運作')
      setLoadFailed(true)
    } finally {
      setLoading(false)
    }
  }

  // Fetch sources on mount
  useEffect(() => {
    let cancelled = false
    setLoading(true)
    listMediaSources()
      .then((data) => { if (!cancelled) setSources(data) })
      .catch(() => { if (!cancelled) { setError('無法載入媒體來源，請確認服務是否正常運作'); setLoadFailed(true) } })
      .finally(() => { if (!cancelled) setLoading(false) })
    return () => { cancelled = true }
  }, [])

  // Check for active import on mount
  useEffect(() => {
    let cancelled = false
    getActiveImportJob().then((job) => {
      if (cancelled || !job) return
      setImportingSourceId(job.source_id)
      setCurrentJobId(job.id)
    }).catch(() => {
      // Non-critical: active job detection failure doesn't block page usage
    })
    return () => { cancelled = true }
  }, [])

  async function handleImport(source: MediaSource) {
    try {
      const job = await importVideos(source.id)
      setImportingSourceId(source.id)
      setCurrentJobId(job.id)
    } catch (err: unknown) {
      const axiosErr = err as { response?: { status?: number; data?: { message?: string } } }
      if (axiosErr?.response?.status === 409) {
        const activeJob = await getActiveImportJob().catch(() => null)
        if (activeJob) {
          setImportingSourceId(activeJob.source_id)
          setCurrentJobId(activeJob.id)
        }
      } else {
        setError(axiosErr?.response?.data?.message || '匯入啟動失敗')
      }
    }
  }

  function handleImportComplete() {
    setImportingSourceId(null)
    setCurrentJobId(null)
    fetchSources()
  }

  async function handleToggleEnabled(source: MediaSource) {
    try {
      await updateMediaSource(source.id, { label: source.label, enabled: !source.enabled })
      setConfirmAction(null)
      fetchSources()
    } catch {
      setError('操作失敗，請重試')
    }
  }

  async function handleDelete(source: MediaSource) {
    try {
      await deleteMediaSource(source.id)
      setConfirmAction(null)
      fetchSources()
    } catch {
      setError('刪除失敗，請重試')
    }
  }

  return (
    <div className="min-h-screen bg-gray-950 flex flex-col">
      <Header searchQuery="" onSearch={() => {}} />

      <div className="flex-1 p-6">
        {/* Top bar */}
        <div className="flex items-center justify-between mb-6">
          <h1 className="text-xl font-bold text-white">媒體來源管理</h1>
          <button
            onClick={() => setShowCreate(true)}
            className="bg-indigo-600 hover:bg-indigo-500 text-white text-sm px-4 py-2 rounded transition-colors"
          >
            + 新增來源
          </button>
        </div>

        {/* Error banner */}
        {error && (
          <div className="bg-red-900/30 border border-red-800 rounded-lg p-3 mb-4 flex items-center justify-between">
            <span className="text-sm text-red-400">{error}</span>
            <button onClick={() => setError(null)} className="text-red-400 hover:text-red-300 text-sm ml-4">✕</button>
          </div>
        )}

        {/* Content */}
        {loading ? (
          <div className="text-gray-500 text-center py-20">載入中...</div>
        ) : loadFailed && sources.length === 0 ? (
          <div className="text-center py-20">
            <p className="text-gray-400 mb-4">無法載入媒體來源</p>
            <button onClick={fetchSources} className="text-indigo-400 hover:text-indigo-300 text-sm">重試</button>
          </div>
        ) : sources.length === 0 ? (
          <div className="text-gray-500 text-center py-20">
            尚未設定媒體來源，請點擊右上角新增
          </div>
        ) : (
          <div className="space-y-4">
            {sources.map((source) => (
              <div
                key={source.id}
                className={`rounded-lg p-4 transition-colors ${
                  source.enabled
                    ? 'bg-gray-900 border border-gray-800'
                    : 'bg-gray-900/50 border border-gray-800/50 opacity-60'
                }`}
              >
                <div className="flex items-start justify-between">
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-2 mb-1">
                      <h3 className="text-white font-medium truncate">{source.label}</h3>
                      <span className={`text-xs px-2 py-0.5 rounded ${
                        source.enabled
                          ? 'bg-green-900/50 text-green-400'
                          : 'bg-gray-800 text-gray-500'
                      }`}>
                        {source.enabled ? '啟用' : '已停用'}
                      </span>
                    </div>
                    <p className="text-sm text-gray-500 font-mono truncate">{source.mount_path}</p>
                    <p className="text-sm text-gray-400 mt-1">{source.video_count} 部影片</p>
                  </div>
                  <div className="flex items-center gap-2 shrink-0 ml-4">
                    <button
                      onClick={() => setEditingSource(source)}
                      className="text-xs text-gray-400 hover:text-white px-2 py-1 rounded transition-colors"
                    >
                      編輯
                    </button>
                    <button
                      onClick={() => handleImport(source)}
                      disabled={importingSourceId !== null}
                      className="text-xs text-indigo-400 hover:text-indigo-300 disabled:opacity-50 disabled:cursor-not-allowed px-2 py-1 rounded transition-colors"
                    >
                      掃描匯入
                    </button>
                    <button
                      onClick={() => setConfirmAction({
                        type: source.enabled ? 'disable' : 'enable',
                        source,
                      })}
                      className={`text-xs px-2 py-1 rounded transition-colors ${
                        source.enabled
                          ? 'text-yellow-400 hover:text-yellow-300'
                          : 'text-green-400 hover:text-green-300'
                      }`}
                    >
                      {source.enabled ? '停用' : '啟用'}
                    </button>
                    <button
                      onClick={() => setConfirmAction({ type: 'delete', source })}
                      className="text-xs text-red-400 hover:text-red-300 px-2 py-1 rounded transition-colors"
                    >
                      刪除
                    </button>
                  </div>
                </div>

                {/* Import progress inline */}
                {importingSourceId === source.id && currentJobId && (
                  <ImportProgress jobId={currentJobId} onComplete={handleImportComplete} />
                )}
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Create Modal */}
      {showCreate && (
        <CreateSourceModal
          onClose={() => setShowCreate(false)}
          onCreated={() => { setShowCreate(false); fetchSources() }}
        />
      )}

      {/* Edit Modal */}
      {editingSource && (
        <EditSourceModal
          source={editingSource}
          onClose={() => setEditingSource(null)}
          onSaved={() => { setEditingSource(null); fetchSources() }}
        />
      )}

      {/* Confirm Dialog */}
      {confirmAction && (
        <ConfirmDialog
          action={confirmAction}
          onConfirm={() => {
            if (confirmAction.type === 'delete') {
              handleDelete(confirmAction.source)
            } else {
              handleToggleEnabled(confirmAction.source)
            }
          }}
          onCancel={() => setConfirmAction(null)}
        />
      )}
    </div>
  )
}

/* ---------- Sub-components ---------- */

function CreateSourceModal({ onClose, onCreated }: { onClose: () => void; onCreated: () => void }) {
  const [label, setLabel] = useState('')
  const [mountPath, setMountPath] = useState('/mnt/host/')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setSubmitting(true)
    setError(null)
    try {
      await createMediaSource({ label: label.trim(), mount_path: mountPath.trim() })
      onCreated()
    } catch (err: unknown) {
      const axiosErr = err as { response?: { status?: number; data?: { message?: string } } }
      const status = axiosErr?.response?.status
      const message = axiosErr?.response?.data?.message
      if (status === 400) {
        setError(message || '路徑驗證失敗，請確認路徑正確')
      } else if (status === 409) {
        setError('此路徑已被其他來源使用')
      } else {
        setError('建立失敗，請重試')
      }
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="fixed inset-0 bg-black/60 flex items-center justify-center z-50" onClick={onClose}>
      <div className="bg-gray-900 rounded-lg p-6 w-full max-w-md" onClick={(e) => e.stopPropagation()}>
        <h2 className="text-lg font-semibold text-white mb-4">新增媒體來源</h2>
        <form onSubmit={handleSubmit}>
          <label className="block text-sm text-gray-400 mb-1">名稱</label>
          <input
            value={label}
            onChange={(e) => setLabel(e.target.value)}
            placeholder="例如：D槽影片"
            className="w-full bg-gray-800 text-white text-sm rounded px-3 py-2 outline-none focus:ring-2 focus:ring-indigo-500 mb-3"
            disabled={submitting}
            required
          />
          <label className="block text-sm text-gray-400 mb-1">路徑</label>
          <input
            value={mountPath}
            onChange={(e) => setMountPath(e.target.value)}
            placeholder="/mnt/host/"
            className="w-full bg-gray-800 text-white text-sm rounded px-3 py-2 outline-none focus:ring-2 focus:ring-indigo-500 mb-1 font-mono"
            disabled={submitting}
            required
          />
          <p className="text-xs text-gray-600 mb-3">路徑必須在 /mnt/host/ 下，對應 Docker 掛載的磁碟目錄</p>

          {error && (
            <div className="bg-red-900/30 border border-red-800 rounded p-2 mb-3">
              <p className="text-sm text-red-400">{error}</p>
            </div>
          )}

          <div className="flex justify-end gap-2">
            <button type="button" onClick={onClose} disabled={submitting} className="text-sm text-gray-400 hover:text-white px-3 py-1.5 rounded">取消</button>
            <button
              type="submit"
              disabled={submitting || !label.trim() || !mountPath.trim()}
              className="bg-indigo-600 hover:bg-indigo-500 disabled:opacity-50 text-white text-sm px-4 py-1.5 rounded"
            >
              {submitting ? '建立中...' : '建立'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}

function EditSourceModal({ source, onClose, onSaved }: { source: MediaSource; onClose: () => void; onSaved: () => void }) {
  const [label, setLabel] = useState(source.label)
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setSubmitting(true)
    setError(null)
    try {
      await updateMediaSource(source.id, { label: label.trim(), enabled: source.enabled })
      onSaved()
    } catch {
      setError('更新失敗，請重試')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="fixed inset-0 bg-black/60 flex items-center justify-center z-50" onClick={onClose}>
      <div className="bg-gray-900 rounded-lg p-6 w-full max-w-md" onClick={(e) => e.stopPropagation()}>
        <h2 className="text-lg font-semibold text-white mb-4">編輯媒體來源</h2>
        <form onSubmit={handleSubmit}>
          <label className="block text-sm text-gray-400 mb-1">名稱</label>
          <input
            value={label}
            onChange={(e) => setLabel(e.target.value)}
            className="w-full bg-gray-800 text-white text-sm rounded px-3 py-2 outline-none focus:ring-2 focus:ring-indigo-500 mb-3"
            disabled={submitting}
            required
          />
          <label className="block text-sm text-gray-400 mb-1">路徑（不可修改）</label>
          <input
            value={source.mount_path}
            disabled
            className="w-full bg-gray-800/50 text-gray-500 text-sm rounded px-3 py-2 mb-1 font-mono cursor-not-allowed"
          />
          <p className="text-xs text-gray-600 mb-3">路徑不可修改。如需變更路徑，請刪除此來源後重新建立。</p>

          {error && (
            <div className="bg-red-900/30 border border-red-800 rounded p-2 mb-3">
              <p className="text-sm text-red-400">{error}</p>
            </div>
          )}

          <div className="flex justify-end gap-2">
            <button type="button" onClick={onClose} disabled={submitting} className="text-sm text-gray-400 hover:text-white px-3 py-1.5 rounded">取消</button>
            <button
              type="submit"
              disabled={submitting || !label.trim()}
              className="bg-indigo-600 hover:bg-indigo-500 disabled:opacity-50 text-white text-sm px-4 py-1.5 rounded"
            >
              {submitting ? '儲存中...' : '儲存'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}

function ConfirmDialog({ action, onConfirm, onCancel }: {
  action: { type: 'disable' | 'enable' | 'delete'; source: MediaSource }
  onConfirm: () => void
  onCancel: () => void
}) {
  const messages: Record<string, string> = {
    disable: `停用後，此來源下的 ${action.source.video_count} 部影片將無法播放。確定要停用嗎？`,
    enable: `確定要重新啟用「${action.source.label}」嗎？`,
    delete: action.source.video_count > 0
      ? `此來源下有 ${action.source.video_count} 部影片記錄，刪除後這些影片將保留在資料庫中但無法播放。確定要刪除嗎？`
      : `確定要刪除「${action.source.label}」嗎？`,
  }

  const isDanger = action.type === 'delete'

  return (
    <div className="fixed inset-0 bg-black/60 flex items-center justify-center z-50" onClick={onCancel}>
      <div className="bg-gray-900 rounded-lg p-6 w-full max-w-sm" onClick={(e) => e.stopPropagation()}>
        <h2 className="text-lg font-semibold text-white mb-2">
          {action.type === 'delete' ? '確認刪除' : action.type === 'disable' ? '確認停用' : '確認啟用'}
        </h2>
        <p className="text-sm text-gray-400 mb-4">{messages[action.type]}</p>
        <div className="flex justify-end gap-2">
          <button onClick={onCancel} className="text-sm text-gray-400 hover:text-white px-3 py-1.5 rounded">取消</button>
          <button
            onClick={onConfirm}
            className={`text-white text-sm px-4 py-1.5 rounded ${
              isDanger ? 'bg-red-600 hover:bg-red-500' : 'bg-indigo-600 hover:bg-indigo-500'
            }`}
          >
            確認
          </button>
        </div>
      </div>
    </div>
  )
}
