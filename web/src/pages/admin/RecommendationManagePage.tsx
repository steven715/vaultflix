import { useState, useEffect } from 'react'
import { Link } from 'react-router-dom'
import { listRecommendationsByDate, createRecommendation, updateRecommendationSortOrder, deleteRecommendation } from '../../api/admin'
import type { RecommendationItem, VideoWithTags } from '../../types'
import Header from '../../components/Header'
import VideoPickerModal from '../../components/admin/VideoPickerModal'
import { formatDuration } from '../../utils/format'

function formatDate(date: Date): string {
  return date.toISOString().slice(0, 10)
}

export default function RecommendationManagePage() {
  const [selectedDate, setSelectedDate] = useState(() => formatDate(new Date()))
  const [recs, setRecs] = useState<RecommendationItem[]>([])
  const [loading, setLoading] = useState(true)

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
    listRecommendationsByDate(selectedDate)
      .then((items) => { if (!cancelled) setRecs(items) })
      .catch(() => { if (!cancelled) setRecs([]) })
      .finally(() => { if (!cancelled) setLoading(false) })
    return () => { cancelled = true }
  }, [selectedDate])

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
    } catch { /* ignore */ }
    finally { setCreating(false) }
  }

  async function handleSortOrderChange(id: string, newOrder: number) {
    if (newOrder < 1) return
    try {
      await updateRecommendationSortOrder(id, newOrder)
      const items = await listRecommendationsByDate(selectedDate)
      setRecs(items)
    } catch { /* ignore */ }
  }

  async function handleDelete(id: string) {
    setDeletingId(null)
    try {
      await deleteRecommendation(id)
      setRecs((prev) => prev.filter((r) => r.id !== id))
    } catch { /* ignore */ }
  }

  return (
    <div className="min-h-screen bg-gray-950 flex flex-col">
      <Header searchQuery="" onSearch={() => {}} />

      <div className="flex-1 p-6">
        {/* Top bar */}
        <div className="flex items-center justify-between mb-6">
          <div className="flex items-center gap-4">
            <Link to="/admin" className="text-sm text-gray-400 hover:text-white transition-colors">
              影片管理
            </Link>
            <h1 className="text-xl font-bold text-white">推薦管理</h1>
          </div>
        </div>

        {/* Date selector */}
        <div className="flex items-center gap-3 mb-6">
          <button onClick={() => shiftDate(-1)} className="bg-gray-800 text-gray-400 hover:text-white px-3 py-1.5 rounded transition-colors">&lt;</button>
          <input
            type="date"
            value={selectedDate}
            onChange={(e) => setSelectedDate(e.target.value)}
            className="bg-gray-800 text-white text-sm rounded px-3 py-1.5 outline-none"
          />
          <button onClick={() => shiftDate(1)} className="bg-gray-800 text-gray-400 hover:text-white px-3 py-1.5 rounded transition-colors">&gt;</button>
          <button onClick={() => setSelectedDate(formatDate(new Date()))} className="text-xs text-indigo-400 hover:text-indigo-300">今天</button>
        </div>

        {/* Recommendations list */}
        {loading ? (
          <div className="text-gray-500 text-center py-20">載入中...</div>
        ) : recs.length === 0 ? (
          <div className="text-gray-500 text-center py-12">此日期尚無推薦</div>
        ) : (
          <div className="overflow-x-auto mb-6">
            <table className="w-full text-sm text-left">
              <thead className="text-gray-400 border-b border-gray-800">
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
                  <tr key={rec.id} className="border-b border-gray-800/50 hover:bg-gray-900/50">
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
                        className="w-14 bg-gray-800 text-gray-300 text-sm text-center rounded px-1 py-1 outline-none focus:ring-1 focus:ring-indigo-500"
                      />
                    </td>
                    <td className="py-2 px-2">
                      <div className="w-16 aspect-video bg-gray-800 rounded overflow-hidden">
                        {rec.thumbnail_url ? (
                          <img src={rec.thumbnail_url} alt="" className="w-full h-full object-cover" loading="lazy" />
                        ) : (
                          <div className="w-full h-full flex items-center justify-center text-gray-600">
                            <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M15.75 10.5l4.72-4.72a.75.75 0 011.28.53v11.38a.75.75 0 01-1.28.53l-4.72-4.72M4.5 18.75h9a2.25 2.25 0 002.25-2.25v-9a2.25 2.25 0 00-2.25-2.25h-9A2.25 2.25 0 002.25 7.5v9a2.25 2.25 0 002.25 2.25z" />
                            </svg>
                          </div>
                        )}
                      </div>
                    </td>
                    <td className="py-2 px-2">
                      <Link to={`/videos/${rec.video_id}`} className="text-white hover:text-indigo-400 transition-colors">
                        {rec.title}
                      </Link>
                    </td>
                    <td className="py-2 px-2 text-gray-400">{formatDuration(rec.duration_seconds)}</td>
                    <td className="py-2 px-2">
                      <button onClick={() => setDeletingId(rec.id)} className="text-xs text-gray-400 hover:text-red-400 transition-colors">移除</button>
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
          className="bg-indigo-600 hover:bg-indigo-500 text-white text-sm px-4 py-2 rounded transition-colors"
        >
          + 新增推薦
        </button>
      </div>

      {/* Video picker modal */}
      {showPicker && (
        <VideoPickerModal
          onSelect={handlePickVideo}
          onClose={() => setShowPicker(false)}
        />
      )}

      {/* Sort order input after picking */}
      {pickedVideo && (
        <div className="fixed inset-0 bg-black/60 flex items-center justify-center z-50" onClick={() => !creating && setPickedVideo(null)}>
          <div className="bg-gray-900 rounded-lg p-6 w-full max-w-sm" onClick={(e) => e.stopPropagation()}>
            <h2 className="text-lg font-semibold text-white mb-3">設定推薦</h2>
            <p className="text-sm text-gray-400 mb-3">
              影片：<span className="text-white">{pickedVideo.title}</span>
            </p>
            <p className="text-sm text-gray-400 mb-1">
              日期：<span className="text-white">{selectedDate}</span>
            </p>
            <label className="block text-sm text-gray-400 mb-1 mt-3">排序順序</label>
            <input
              type="number"
              value={sortOrder}
              onChange={(e) => setSortOrder(Number(e.target.value))}
              min={1}
              className="w-full bg-gray-800 text-white text-sm rounded px-3 py-2 outline-none focus:ring-2 focus:ring-indigo-500 mb-4"
              disabled={creating}
            />
            <div className="flex justify-end gap-2">
              <button onClick={() => setPickedVideo(null)} disabled={creating} className="text-sm text-gray-400 hover:text-white px-3 py-1.5 rounded">取消</button>
              <button onClick={handleCreate} disabled={creating} className="bg-indigo-600 hover:bg-indigo-500 disabled:opacity-50 text-white text-sm px-4 py-1.5 rounded">
                {creating ? '建立中...' : '確認'}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Delete confirm */}
      {deletingId && (
        <div className="fixed inset-0 bg-black/60 flex items-center justify-center z-50" onClick={() => setDeletingId(null)}>
          <div className="bg-gray-900 rounded-lg p-6 w-full max-w-sm" onClick={(e) => e.stopPropagation()}>
            <h2 className="text-lg font-semibold text-white mb-2">確認移除</h2>
            <p className="text-sm text-gray-400 mb-4">確定要移除此推薦嗎？</p>
            <div className="flex justify-end gap-2">
              <button onClick={() => setDeletingId(null)} className="text-sm text-gray-400 hover:text-white px-3 py-1.5 rounded">取消</button>
              <button onClick={() => handleDelete(deletingId)} className="bg-red-600 hover:bg-red-500 text-white text-sm px-4 py-1.5 rounded">移除</button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
