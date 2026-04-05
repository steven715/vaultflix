import { useState, useEffect } from 'react'
import { Link } from 'react-router-dom'
import { listFavorites, removeFavorite } from '../api/favorites'
import type { VideoSummaryWithURL } from '../types'
import Pagination from '../components/Pagination'

function formatDuration(seconds: number): string {
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  const s = seconds % 60
  if (h > 0) {
    return `${h}:${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`
  }
  return `${m}:${String(s).padStart(2, '0')}`
}

function formatFileSize(bytes: number): string {
  if (bytes >= 1073741824) return `${(bytes / 1073741824).toFixed(1)} GB`
  if (bytes >= 1048576) return `${(bytes / 1048576).toFixed(0)} MB`
  return `${(bytes / 1024).toFixed(0)} KB`
}

const PAGE_SIZE = 20

export default function FavoritesPage() {
  const [favorites, setFavorites] = useState<VideoSummaryWithURL[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [loading, setLoading] = useState(true)

  const totalPages = Math.ceil(total / PAGE_SIZE)

  useEffect(() => {
    let cancelled = false
    setLoading(true)

    listFavorites(page, PAGE_SIZE)
      .then((res) => {
        if (cancelled) return
        setFavorites(res.data)
        setTotal(res.total)
      })
      .catch(() => {
        if (cancelled) return
        setFavorites([])
        setTotal(0)
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })

    return () => { cancelled = true }
  }, [page])

  async function handleRemove(videoID: string) {
    // Optimistic UI: remove immediately
    setFavorites((prev) => prev.filter((f) => f.id !== videoID))
    setTotal((prev) => prev - 1)

    try {
      await removeFavorite(videoID)
    } catch {
      // Rollback: re-fetch on failure
      listFavorites(page, PAGE_SIZE)
        .then((res) => {
          setFavorites(res.data)
          setTotal(res.total)
        })
        .catch(() => {})
    }
  }

  return (
    <div className="min-h-screen bg-gray-950">
      <div className="max-w-6xl mx-auto px-4 py-6">
        <div className="flex items-center justify-between mb-6">
          <h1 className="text-xl font-semibold text-white">我的收藏</h1>
          <Link to="/" className="text-sm text-gray-400 hover:text-white transition-colors">
            ← 返回瀏覽
          </Link>
        </div>

        {loading ? (
          <div className="flex items-center justify-center py-20 text-gray-500">
            載入中...
          </div>
        ) : favorites.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-20 gap-3">
            <div className="text-gray-500">尚無收藏影片</div>
            <Link to="/" className="text-indigo-400 hover:text-indigo-300 text-sm">
              去瀏覽影片
            </Link>
          </div>
        ) : (
          <>
            <div className="text-sm text-gray-500 mb-4">共 {total} 部收藏</div>
            <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-4">
              {favorites.map((video) => (
                <div key={video.id} className="group relative bg-gray-900 rounded-lg overflow-hidden">
                  <Link to={`/videos/${video.id}`}>
                    <div className="aspect-video bg-gray-800 relative">
                      {video.thumbnail_url ? (
                        <img
                          src={video.thumbnail_url}
                          alt={video.title}
                          className="w-full h-full object-cover"
                          loading="lazy"
                        />
                      ) : (
                        <div className="w-full h-full flex items-center justify-center text-gray-600">
                          <svg className="w-12 h-12" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M15.75 10.5l4.72-4.72a.75.75 0 011.28.53v11.38a.75.75 0 01-1.28.53l-4.72-4.72M4.5 18.75h9a2.25 2.25 0 002.25-2.25v-9a2.25 2.25 0 00-2.25-2.25h-9A2.25 2.25 0 002.25 7.5v9a2.25 2.25 0 002.25 2.25z" />
                          </svg>
                        </div>
                      )}
                      <span className="absolute bottom-1 right-1 bg-black/80 text-white text-xs px-1.5 py-0.5 rounded">
                        {formatDuration(video.duration_seconds)}
                      </span>
                    </div>
                    <div className="p-3">
                      <h3 className="text-sm text-white font-medium line-clamp-2 group-hover:text-indigo-400 transition-colors">
                        {video.title}
                      </h3>
                      <div className="mt-1 flex items-center gap-2 text-xs text-gray-500">
                        <span>{video.resolution}</span>
                        <span>·</span>
                        <span>{formatFileSize(video.file_size_bytes)}</span>
                      </div>
                    </div>
                  </Link>

                  {/* Remove favorite button */}
                  <button
                    onClick={(e) => { e.preventDefault(); handleRemove(video.id) }}
                    className="absolute top-2 right-2 p-1.5 bg-black/60 rounded-full opacity-0 group-hover:opacity-100 transition-opacity hover:bg-black/80"
                    title="取消收藏"
                  >
                    <svg className="w-4 h-4 text-red-500 fill-red-500" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                      <path strokeLinecap="round" strokeLinejoin="round" d="M21 8.25c0-2.485-2.099-4.5-4.688-4.5-1.935 0-3.597 1.126-4.312 2.733-.715-1.607-2.377-2.733-4.313-2.733C5.1 3.75 3 5.765 3 8.25c0 7.22 9 12 9 12s9-4.78 9-12z" />
                    </svg>
                  </button>
                </div>
              ))}
            </div>

            <Pagination page={page} totalPages={totalPages} onPageChange={setPage} />
          </>
        )}
      </div>
    </div>
  )
}
