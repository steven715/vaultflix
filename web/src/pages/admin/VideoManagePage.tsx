import { useState, useEffect, useCallback, useRef } from 'react'
import { useSearchParams, Link } from 'react-router-dom'
import { listVideos } from '../../api/videos'
import { listTags } from '../../api/tags'
import { importVideos, updateVideo, deleteVideo } from '../../api/admin'
import type { VideoWithTags, TagWithCount, ImportResult } from '../../types'
import Header from '../../components/Header'
import Pagination from '../../components/Pagination'
import TagInput from '../../components/TagInput'
import { formatDuration, formatFileSize } from '../../utils/format'

export default function VideoManagePage() {
  const [searchParams, setSearchParams] = useSearchParams()
  const [videos, setVideos] = useState<VideoWithTags[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [allTags, setAllTags] = useState<TagWithCount[]>([])

  // Import modal state
  const [showImport, setShowImport] = useState(false)
  const [importDir, setImportDir] = useState('')
  const [importing, setImporting] = useState(false)
  const [importResult, setImportResult] = useState<ImportResult | null>(null)

  // Edit modal state
  const [editingVideo, setEditingVideo] = useState<VideoWithTags | null>(null)
  const [editTitle, setEditTitle] = useState('')
  const [editDesc, setEditDesc] = useState('')
  const [saving, setSaving] = useState(false)

  // Delete confirm state
  const [deletingVideo, setDeletingVideo] = useState<VideoWithTags | null>(null)


  const page = Number(searchParams.get('page')) || 1
  const pageSize = Number(searchParams.get('page_size')) || 20
  const query = searchParams.get('q') || ''
  const tagIdsStr = searchParams.get('tag_ids') || ''
  const totalPages = Math.ceil(total / pageSize)

  const [searchInput, setSearchInput] = useState(query)
  const debounceRef = useRef<ReturnType<typeof setTimeout>>(undefined)

  useEffect(() => { setSearchInput(query) }, [query])

  const updateParams = useCallback((updates: Record<string, string>) => {
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev)
      for (const [key, value] of Object.entries(updates)) {
        if (value) next.set(key, value)
        else next.delete(key)
      }
      return next
    })
  }, [setSearchParams])

  function handleSearch(value: string) {
    setSearchInput(value)
    clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(() => {
      updateParams({ q: value, page: '1' })
    }, 300)
  }

  // Fetch videos
  useEffect(() => {
    let cancelled = false
    setLoading(true)
    listVideos({ page, page_size: pageSize, sort_by: 'created_at', sort_order: 'desc', q: query || undefined, tag_ids: tagIdsStr || undefined })
      .then((res) => {
        if (cancelled) return
        setVideos(res.data)
        setTotal(res.total)
      })
      .catch(() => { if (!cancelled) { setVideos([]); setTotal(0) } })
      .finally(() => { if (!cancelled) setLoading(false) })
    return () => { cancelled = true }
  }, [page, pageSize, query, tagIdsStr])

  // Fetch tags
  useEffect(() => {
    listTags().then(setAllTags).catch(() => {})
  }, [])

  // Import handler
  async function handleImport() {
    if (!importDir.trim()) return
    setImporting(true)
    setImportResult(null)
    try {
      const result = await importVideos(importDir.trim())
      setImportResult(result)
      // Refresh video list
      updateParams({ page: '1' })
    } catch {
      setImportResult({ total_scanned: 0, imported: 0, skipped: 0, failed: 0, failures: [{ filename: '', error: 'import request failed' }] })
    } finally {
      setImporting(false)
    }
  }

  // Edit handler
  function openEdit(video: VideoWithTags) {
    setEditingVideo(video)
    setEditTitle(video.title)
    setEditDesc(video.description)
  }

  async function handleSaveEdit() {
    if (!editingVideo) return
    setSaving(true)
    try {
      const updated = await updateVideo(editingVideo.id, { title: editTitle, description: editDesc })
      setVideos((prev) => prev.map((v) => v.id === updated.id ? { ...v, title: updated.title, description: updated.description } : v))
      setEditingVideo(null)
    } catch { /* keep modal open on error */ }
    finally { setSaving(false) }
  }

  // Delete handler
  async function handleDelete() {
    if (!deletingVideo) return
    const id = deletingVideo.id
    setDeletingVideo(null)
    try {
      await deleteVideo(id)
      setVideos((prev) => prev.filter((v) => v.id !== id))
      setTotal((prev) => prev - 1)
    } catch { /* silently fail, could add error toast */ }
  }


  return (
    <div className="min-h-screen bg-gray-950 flex flex-col">
      <Header searchQuery={searchInput} onSearch={handleSearch} />

      <div className="flex-1 p-6">
        {/* Top bar */}
        <div className="flex items-center justify-between mb-6">
          <div className="flex items-center gap-4">
            <h1 className="text-xl font-bold text-white">影片管理</h1>
            <Link to="/admin/recommendations" className="text-sm text-gray-400 hover:text-white transition-colors">
              推薦管理
            </Link>
          </div>
          <button
            onClick={() => { setShowImport(true); setImportResult(null); setImportDir('') }}
            className="bg-indigo-600 hover:bg-indigo-500 text-white text-sm px-4 py-2 rounded transition-colors"
          >
            匯入影片
          </button>
        </div>

        {/* Video table */}
        {loading ? (
          <div className="text-gray-500 text-center py-20">載入中...</div>
        ) : videos.length === 0 ? (
          <div className="text-gray-500 text-center py-20">
            {query || tagIdsStr ? '沒有符合條件的影片' : '尚無影片'}
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm text-left">
              <thead className="text-gray-400 border-b border-gray-800">
                <tr>
                  <th className="py-3 px-2 w-24">縮圖</th>
                  <th className="py-3 px-2">標題</th>
                  <th className="py-3 px-2 w-20">時長</th>
                  <th className="py-3 px-2 w-20">大小</th>
                  <th className="py-3 px-2 w-48">標籤</th>
                  <th className="py-3 px-2 w-28">操作</th>
                </tr>
              </thead>
              <tbody>
                {videos.map((video) => (
                  <tr key={video.id} className="border-b border-gray-800/50 hover:bg-gray-900/50">
                    <td className="py-2 px-2">
                      <div className="w-20 aspect-video bg-gray-800 rounded overflow-hidden">
                        {video.thumbnail_url ? (
                          <img src={video.thumbnail_url} alt="" className="w-full h-full object-cover" loading="lazy" />
                        ) : (
                          <div className="w-full h-full flex items-center justify-center text-gray-600">
                            <svg className="w-6 h-6" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M15.75 10.5l4.72-4.72a.75.75 0 011.28.53v11.38a.75.75 0 01-1.28.53l-4.72-4.72M4.5 18.75h9a2.25 2.25 0 002.25-2.25v-9a2.25 2.25 0 00-2.25-2.25h-9A2.25 2.25 0 002.25 7.5v9a2.25 2.25 0 002.25 2.25z" />
                            </svg>
                          </div>
                        )}
                      </div>
                    </td>
                    <td className="py-2 px-2">
                      <Link to={`/videos/${video.id}`} className="text-white hover:text-indigo-400 transition-colors">
                        {video.title}
                      </Link>
                      {video.description && (
                        <p className="text-xs text-gray-500 mt-0.5 line-clamp-1">{video.description}</p>
                      )}
                    </td>
                    <td className="py-2 px-2 text-gray-400">{formatDuration(video.duration_seconds)}</td>
                    <td className="py-2 px-2 text-gray-400">{formatFileSize(video.file_size_bytes)}</td>
                    <td className="py-2 px-2">
                      <TagInput
                        videoId={video.id}
                        initialTags={video.tags}
                        allTags={allTags}
                        onTagsChange={() => {
                          listTags().then(setAllTags).catch(() => {})
                        }}
                      />
                    </td>
                    <td className="py-2 px-2">
                      <div className="flex gap-2">
                        <button onClick={() => openEdit(video)} className="text-xs text-gray-400 hover:text-white transition-colors">編輯</button>
                        <button onClick={() => setDeletingVideo(video)} className="text-xs text-gray-400 hover:text-red-400 transition-colors">刪除</button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}

        <Pagination page={page} totalPages={totalPages} onPageChange={(p) => updateParams({ page: String(p) })} />
      </div>

      {/* Import Modal */}
      {showImport && (
        <div className="fixed inset-0 bg-black/60 flex items-center justify-center z-50" onClick={() => !importing && setShowImport(false)}>
          <div className="bg-gray-900 rounded-lg p-6 w-full max-w-md" onClick={(e) => e.stopPropagation()}>
            <h2 className="text-lg font-semibold text-white mb-4">匯入影片</h2>
            {!importResult ? (
              <>
                <input
                  value={importDir}
                  onChange={(e) => setImportDir(e.target.value)}
                  placeholder="來源目錄路徑 (例: /data/videos)"
                  className="w-full bg-gray-800 text-white text-sm rounded px-3 py-2 outline-none focus:ring-2 focus:ring-indigo-500 mb-4"
                  disabled={importing}
                />
                <div className="flex justify-end gap-2">
                  <button onClick={() => setShowImport(false)} disabled={importing} className="text-sm text-gray-400 hover:text-white px-3 py-1.5 rounded">取消</button>
                  <button onClick={handleImport} disabled={importing || !importDir.trim()} className="bg-indigo-600 hover:bg-indigo-500 disabled:opacity-50 text-white text-sm px-4 py-1.5 rounded">
                    {importing ? '匯入中...' : '開始匯入'}
                  </button>
                </div>
              </>
            ) : (
              <>
                <div className="space-y-2 text-sm mb-4">
                  <div className="flex justify-between text-gray-300"><span>掃描檔案</span><span>{importResult.total_scanned}</span></div>
                  <div className="flex justify-between text-green-400"><span>成功匯入</span><span>{importResult.imported}</span></div>
                  <div className="flex justify-between text-gray-400"><span>已跳過（重複）</span><span>{importResult.skipped}</span></div>
                  <div className="flex justify-between text-red-400"><span>失敗</span><span>{importResult.failed}</span></div>
                </div>
                {importResult.failures.length > 0 && (
                  <div className="mb-4">
                    <h3 className="text-xs font-medium text-red-400 mb-1">失敗詳情</h3>
                    <div className="bg-gray-800 rounded p-2 max-h-40 overflow-y-auto space-y-1">
                      {importResult.failures.map((f, i) => (
                        <div key={i} className="text-xs">
                          <span className="text-gray-300">{f.filename}</span>
                          <span className="text-gray-600 ml-1">— {f.error}</span>
                        </div>
                      ))}
                    </div>
                  </div>
                )}
                <div className="flex justify-end">
                  <button onClick={() => setShowImport(false)} className="bg-indigo-600 hover:bg-indigo-500 text-white text-sm px-4 py-1.5 rounded">關閉</button>
                </div>
              </>
            )}
          </div>
        </div>
      )}

      {/* Edit Modal */}
      {editingVideo && (
        <div className="fixed inset-0 bg-black/60 flex items-center justify-center z-50" onClick={() => !saving && setEditingVideo(null)}>
          <div className="bg-gray-900 rounded-lg p-6 w-full max-w-md" onClick={(e) => e.stopPropagation()}>
            <h2 className="text-lg font-semibold text-white mb-4">編輯影片</h2>
            <label className="block text-sm text-gray-400 mb-1">標題</label>
            <input
              value={editTitle}
              onChange={(e) => setEditTitle(e.target.value)}
              className="w-full bg-gray-800 text-white text-sm rounded px-3 py-2 outline-none focus:ring-2 focus:ring-indigo-500 mb-3"
              disabled={saving}
            />
            <label className="block text-sm text-gray-400 mb-1">描述</label>
            <textarea
              value={editDesc}
              onChange={(e) => setEditDesc(e.target.value)}
              rows={3}
              className="w-full bg-gray-800 text-white text-sm rounded px-3 py-2 outline-none focus:ring-2 focus:ring-indigo-500 mb-4 resize-none"
              disabled={saving}
            />
            <div className="flex justify-end gap-2">
              <button onClick={() => setEditingVideo(null)} disabled={saving} className="text-sm text-gray-400 hover:text-white px-3 py-1.5 rounded">取消</button>
              <button onClick={handleSaveEdit} disabled={saving || !editTitle.trim()} className="bg-indigo-600 hover:bg-indigo-500 disabled:opacity-50 text-white text-sm px-4 py-1.5 rounded">
                {saving ? '儲存中...' : '儲存'}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Delete Confirm */}
      {deletingVideo && (
        <div className="fixed inset-0 bg-black/60 flex items-center justify-center z-50" onClick={() => setDeletingVideo(null)}>
          <div className="bg-gray-900 rounded-lg p-6 w-full max-w-sm" onClick={(e) => e.stopPropagation()}>
            <h2 className="text-lg font-semibold text-white mb-2">確認刪除</h2>
            <p className="text-sm text-gray-400 mb-4">
              確定要刪除「<span className="text-white">{deletingVideo.title}</span>」嗎？此操作無法復原。
            </p>
            <div className="flex justify-end gap-2">
              <button onClick={() => setDeletingVideo(null)} className="text-sm text-gray-400 hover:text-white px-3 py-1.5 rounded">取消</button>
              <button onClick={handleDelete} className="bg-red-600 hover:bg-red-500 text-white text-sm px-4 py-1.5 rounded">刪除</button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
