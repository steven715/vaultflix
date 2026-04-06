import { useState, useEffect } from 'react'
import { Link } from 'react-router-dom'
import { listWatchHistory } from '../api/watchHistory'
import type { WatchHistoryItem } from '../types'
import Pagination from '../components/Pagination'
import { formatDuration } from '../utils/format'

function formatRelativeTime(dateStr: string): string {
  const date = new Date(dateStr)
  const now = new Date()
  const diffMs = now.getTime() - date.getTime()
  const diffMin = Math.floor(diffMs / 60000)
  const diffHr = Math.floor(diffMin / 60)
  const diffDay = Math.floor(diffHr / 24)

  if (diffMin < 1) return '剛剛'
  if (diffMin < 60) return `${diffMin} 分鐘前`
  if (diffHr < 24) return `${diffHr} 小時前`
  if (diffDay < 7) return `${diffDay} 天前`
  return date.toLocaleDateString('zh-TW', { month: 'short', day: 'numeric' })
}

const PAGE_SIZE = 20

export default function HistoryPage() {
  const [items, setItems] = useState<WatchHistoryItem[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [loading, setLoading] = useState(true)

  const totalPages = Math.ceil(total / PAGE_SIZE)

  useEffect(() => {
    let cancelled = false
    setLoading(true)

    listWatchHistory(page, PAGE_SIZE)
      .then((res) => {
        if (cancelled) return
        setItems(res.data)
        setTotal(res.total)
      })
      .catch(() => {
        if (cancelled) return
        setItems([])
        setTotal(0)
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })

    return () => { cancelled = true }
  }, [page])

  return (
    <div className="min-h-screen bg-gray-950">
      <div className="max-w-4xl mx-auto px-4 py-6">
        <div className="flex items-center justify-between mb-6">
          <h1 className="text-xl font-semibold text-white">觀看記錄</h1>
          <Link to="/" className="text-sm text-gray-400 hover:text-white transition-colors">
            ← 返回瀏覽
          </Link>
        </div>

        {loading ? (
          <div className="flex items-center justify-center py-20 text-gray-500">
            載入中...
          </div>
        ) : items.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-20 gap-3">
            <div className="text-gray-500">尚無觀看記錄</div>
            <Link to="/" className="text-indigo-400 hover:text-indigo-300 text-sm">
              去瀏覽影片
            </Link>
          </div>
        ) : (
          <>
            <div className="space-y-2">
              {items.map((item) => {
                const percent = item.duration_seconds > 0
                  ? Math.min(100, Math.round((item.progress_seconds / item.duration_seconds) * 100))
                  : 0

                return (
                  <Link
                    key={item.id}
                    to={`/videos/${item.video_id}`}
                    className="flex items-center gap-4 p-3 bg-gray-900 rounded-lg hover:bg-gray-800 transition-colors group"
                  >
                    {/* Thumbnail */}
                    <div className="w-32 shrink-0 aspect-video bg-gray-800 rounded overflow-hidden relative">
                      {item.thumbnail_url ? (
                        <img
                          src={item.thumbnail_url}
                          alt={item.title}
                          className="w-full h-full object-cover"
                          loading="lazy"
                        />
                      ) : (
                        <div className="w-full h-full flex items-center justify-center text-gray-600">
                          <svg className="w-8 h-8" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M15.75 10.5l4.72-4.72a.75.75 0 011.28.53v11.38a.75.75 0 01-1.28.53l-4.72-4.72M4.5 18.75h9a2.25 2.25 0 002.25-2.25v-9a2.25 2.25 0 00-2.25-2.25h-9A2.25 2.25 0 002.25 7.5v9a2.25 2.25 0 002.25 2.25z" />
                          </svg>
                        </div>
                      )}
                      {/* Progress bar overlay */}
                      <div className="absolute bottom-0 left-0 right-0 h-1 bg-gray-700">
                        <div
                          className="h-full bg-indigo-500"
                          style={{ width: `${percent}%` }}
                        />
                      </div>
                    </div>

                    {/* Info */}
                    <div className="flex-1 min-w-0">
                      <h3 className="text-sm text-white font-medium truncate group-hover:text-indigo-400 transition-colors">
                        {item.title}
                      </h3>
                      <div className="mt-1 flex items-center gap-3 text-xs text-gray-500">
                        <span>{formatRelativeTime(item.watched_at)}</span>
                        <span>
                          {item.completed
                            ? '已看完'
                            : `${formatDuration(item.progress_seconds)} / ${formatDuration(item.duration_seconds)}`}
                        </span>
                        <span>{percent}%</span>
                      </div>
                    </div>
                  </Link>
                )
              })}
            </div>

            <Pagination page={page} totalPages={totalPages} onPageChange={setPage} />
          </>
        )}
      </div>
    </div>
  )
}
