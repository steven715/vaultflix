import { useState, useEffect } from 'react'
import { Link } from 'react-router-dom'
import { listRecommendationsByDate, createRecommendation, updateRecommendationSortOrder, deleteRecommendation } from '../../api/admin'
import type { RecommendationItem, VideoWithTags } from '../../types'
import PosterThumb from '../../components/admin/PosterThumb'
import VideoPickerModal from '../../components/admin/VideoPickerModal'
import ErrorBanner from '../../components/ErrorBanner'
import { useToast } from '../../contexts/ToastContext'
import { formatDuration } from '../../utils/format'

function formatDate(date: Date): string {
  return date.toISOString().slice(0, 10)
}

export default function RecommendationManagePage() {
  const [selectedDate, setSelectedDate] = useState(() => formatDate(new Date()))
  const [recs, setRecs] = useState<RecommendationItem[]>([])
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState(false)
  const [reloadKey, setReloadKey] = useState(0)
  const toast = useToast()

  // Add modal state
  const [showPicker, setShowPicker] = useState(false)
  const [pickedVideo, setPickedVideo] = useState<VideoWithTags | null>(null)
  const [sortOrder, setSortOrder] = useState(1)
  const [creating, setCreating] = useState(false)

  // Delete confirm state
  const [deletingId, setDeletingId] = useState<string | null>(null)

  function shiftDate(days: number) {
    const d = new Date(selectedDate + 'T00:00:00')
    d.setDate(d.getDate() + days)
    setSelectedDate(formatDate(d))
  }

  // Fetch recommendations for selected date
  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setLoadError(false)
    listRecommendationsByDate(selectedDate)
      .then((items) => { if (!cancelled) setRecs(items) })
      .catch(() => { if (!cancelled) { setRecs([]); setLoadError(true) } })
      .finally(() => { if (!cancelled) setLoading(false) })
    return () => { cancelled = true }
  }, [selectedDate, reloadKey])

  function handlePickVideo(video: VideoWithTags) {
    setPickedVideo(video)
    setShowPicker(false)
    setSortOrder(recs.length + 1)
  }

  async function handleCreate() {
    if (!pickedVideo) return
    setCreating(true)
    try {
      await createRecommendation(pickedVideo.id, selectedDate, sortOrder)
      setPickedVideo(null)
      // Refresh list
      const items = await listRecommendationsByDate(selectedDate)
      setRecs(items)
    } catch {
      toast.error('新增推薦失敗，請重試')
    } finally { setCreating(false) }
  }

  async function handleSortOrderChange(id: string, newOrder: number) {
    if (newOrder < 1) return
    try {
      await updateRecommendationSortOrder(id, newOrder)
      const items = await listRecommendationsByDate(selectedDate)
      setRecs(items)
    } catch {
      toast.error('更新排序失敗，請重試')
    }
  }

  async function handleDelete(id: string) {
    setDeletingId(null)
    try {
      await deleteRecommendation(id)
      setRecs((prev) => prev.filter((r) => r.id !== id))
    } catch {
      toast.error('移除推薦失敗，請重試')
    }
  }

  return (
    <div className="p-7">
      {/* Top bar */}
      <div className="flex items-center justify-between mb-6">
        <div className="flex items-center gap-4">
          <Link to="/admin/library" className="text-sm text-muted hover:text-cream transition-colors">
            影片庫
          </Link>
          <h1 className="font-display font-bold tracking-tight text-cream text-xl">推薦管理</h1>
        </div>
      </div>

      {/* Date selector */}
      <div className="flex items-center gap-3 mb-6">
        <button onClick={() => shiftDate(-1)} className="bg-surface-2 text-muted hover:text-cream px-3 py-1.5 rounded transition-colors">&lt;</button>
        <input
          type="date"
          value={selectedDate}
          onChange={(e) => setSelectedDate(e.target.value)}
          className="bg-surface-2 text-cream text-sm rounded px-3 py-1.5 outline-none focus:ring-1 focus:ring-accent"
        />
        <button onClick={() => shiftDate(1)} className="bg-surface-2 text-muted hover:text-cream px-3 py-1.5 rounded transition-colors">&gt;</button>
        <button onClick={() => setSelectedDate(formatDate(new Date()))} className="text-xs text-accent hover:text-accent/80">今天</button>
      </div>

      {/* Recommendations list */}
      {loading ? (
        <div className="text-muted text-center py-20">載入中...</div>
      ) : loadError ? (
        <ErrorBanner
          message="無法載入推薦，請確認服務是否正常運作"
          onRetry={() => setReloadKey((k) => k + 1)}
        />
      ) : recs.length === 0 ? (
        <div className="text-muted text-center py-12">此日期尚無推薦</div>
      ) : (
        <div className="overflow-x-auto mb-6">
          <table className="w-full text-sm text-left">
            <thead className="bg-surface-2 text-muted border-b border-border">
              <tr>
                <th className="py-3 px-2 w-20">排序</th>
                <th className="py-3 px-2 w-24">縮圖</th>
                <th className="py-3 px-2">標題</th>
                <th className="py-3 px-2 w-20">時長</th>
                <th className="py-3 px-2 w-20">操作</th>
              </tr>
            </thead>
            <tbody>
              {recs.map((rec) => (
                <tr key={rec.id} className="border-b border-border hover:bg-surface-2">
                  <td className="py-2 px-2">
                    <input
                      type="number"
                      defaultValue={rec.sort_order}
                      min={1}
                      onBlur={(e) => {
                        const val = Number(e.target.value)
                        if (val !== rec.sort_order && val >= 1) handleSortOrderChange(rec.id, val)
                      }}
                      onKeyDown={(e) => {
                        if (e.key === 'Enter') (e.target as HTMLInputElement).blur()
                      }}
                      className="w-14 bg-surface-2 text-muted text-sm text-center rounded px-1 py-1 outline-none focus:ring-1 focus:ring-accent"
                    />
                  </td>
                  <td className="py-2 px-2">
                    <PosterThumb id={rec.video_id} src={rec.thumbnail_url} className="w-16" />
                  </td>
                  <td className="py-2 px-2">
                    <Link to={`/videos/${rec.video_id}`} className="text-cream hover:text-accent transition-colors">
                      {rec.title}
                    </Link>
                  </td>
                  <td className="py-2 px-2 font-mono text-muted">{formatDuration(rec.duration_seconds)}</td>
                  <td className="py-2 px-2">
                    <button onClick={() => setDeletingId(rec.id)} className="text-xs text-muted hover:text-fav transition-colors">移除</button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* Add recommendation */}
      <button
        onClick={() => setShowPicker(true)}
        className="bg-accent text-accent-ink hover:brightness-110 text-sm px-4 py-2 rounded transition-colors"
      >
        + 新增推薦
      </button>

      {/* Video picker modal */}
      {showPicker && (
        <VideoPickerModal
          onSelect={handlePickVideo}
          onClose={() => setShowPicker(false)}
        />
      )}

      {/* Sort order input after picking */}
      {pickedVideo && (
        <div className="fixed inset-0 bg-[rgba(8,6,5,0.72)] backdrop-blur-[3px] flex items-center justify-center z-50" onClick={() => !creating && setPickedVideo(null)}>
          <div className="bg-surface border border-border rounded-lg p-6 w-full max-w-sm" onClick={(e) => e.stopPropagation()}>
            <h2 className="font-display font-bold tracking-tight text-cream text-lg mb-3">設定推薦</h2>
            <p className="text-sm text-muted mb-3">
              影片：<span className="text-cream">{pickedVideo.title}</span>
            </p>
            <p className="text-sm text-muted mb-1">
              日期：<span className="text-cream">{selectedDate}</span>
            </p>
            <label className="block text-sm text-muted mb-1 mt-3">排序順序</label>
            <input
              type="number"
              value={sortOrder}
              onChange={(e) => setSortOrder(Number(e.target.value))}
              min={1}
              className="w-full bg-surface-2 text-cream text-sm rounded px-3 py-2 outline-none focus:ring-2 focus:ring-accent mb-4"
              disabled={creating}
            />
            <div className="flex justify-end gap-2">
              <button onClick={() => setPickedVideo(null)} disabled={creating} className="text-sm text-muted hover:text-cream px-3 py-1.5 rounded">取消</button>
              <button onClick={handleCreate} disabled={creating} className="bg-accent text-accent-ink hover:brightness-110 disabled:opacity-50 text-sm px-4 py-1.5 rounded">
                {creating ? '建立中...' : '確認'}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Delete confirm */}
      {deletingId && (
        <div className="fixed inset-0 bg-[rgba(8,6,5,0.72)] backdrop-blur-[3px] flex items-center justify-center z-50" onClick={() => setDeletingId(null)}>
          <div className="bg-surface border border-border rounded-lg p-6 w-full max-w-sm" onClick={(e) => e.stopPropagation()}>
            <h2 className="font-display font-bold tracking-tight text-cream text-lg mb-2">確認移除</h2>
            <p className="text-sm text-muted mb-4">確定要移除此推薦嗎？</p>
            <div className="flex justify-end gap-2">
              <button onClick={() => setDeletingId(null)} className="text-sm text-muted hover:text-cream px-3 py-1.5 rounded">取消</button>
              <button onClick={() => handleDelete(deletingId)} className="bg-fav text-cream hover:brightness-110 text-sm px-4 py-1.5 rounded">移除</button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
