import { useState, useEffect, useCallback, useRef } from 'react'
import { useSearchParams, Link } from 'react-router-dom'
import { listVideos } from '../api/videos'
import { getTodayRecommendations } from '../api/recommendations'
import type { VideoWithTags, RecommendationItem } from '../types'
import Header from '../components/Header'
import TagSidebar from '../components/TagSidebar'
import VideoCard from '../components/VideoCard'
import Pagination from '../components/Pagination'
import { formatDuration } from '../utils/format'

const SORT_OPTIONS = [
  { value: 'created_at', label: '最新' },
  { value: 'title', label: '標題' },
  { value: 'duration_seconds', label: '時長' },
  { value: 'file_size_bytes', label: '檔案大小' },
]

export default function BrowsePage() {
  const [searchParams, setSearchParams] = useSearchParams()
  const [videos, setVideos] = useState<VideoWithTags[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [recommendations, setRecommendations] = useState<RecommendationItem[]>([])
  const [recsLoaded, setRecsLoaded] = useState(false)

  const page = Number(searchParams.get('page')) || 1
  const pageSize = Number(searchParams.get('page_size')) || 20
  const sortBy = searchParams.get('sort_by') || 'created_at'
  const sortOrder = searchParams.get('sort_order') || 'desc'
  const query = searchParams.get('q') || ''
  const tagIdsStr = searchParams.get('tag_ids') || ''
  const selectedTagIds = tagIdsStr ? tagIdsStr.split(',').map(Number) : []

  const totalPages = Math.ceil(total / pageSize)

  // Debounce search input
  const [searchInput, setSearchInput] = useState(query)
  const debounceRef = useRef<ReturnType<typeof setTimeout>>(undefined)

  useEffect(() => {
    setSearchInput(query)
  }, [query])

  const updateParams = useCallback(
    (updates: Record<string, string>) => {
      setSearchParams((prev) => {
        const next = new URLSearchParams(prev)
        for (const [key, value] of Object.entries(updates)) {
          if (value) {
            next.set(key, value)
          } else {
            next.delete(key)
          }
        }
        return next
      })
    },
    [setSearchParams],
  )

  function handleSearch(value: string) {
    setSearchInput(value)
    clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(() => {
      updateParams({ q: value, page: '1' })
    }, 300)
  }

  function handleTagsChange(tagIds: number[]) {
    updateParams({ tag_ids: tagIds.join(','), page: '1' })
  }

  function handlePageChange(newPage: number) {
    updateParams({ page: String(newPage) })
  }

  function handleSortChange(newSortBy: string) {
    updateParams({ sort_by: newSortBy, page: '1' })
  }

  function handleSortOrderToggle() {
    updateParams({ sort_order: sortOrder === 'asc' ? 'desc' : 'asc', page: '1' })
  }

  const [recsKey, setRecsKey] = useState(0)

  // Fetch today's recommendations (re-fetch when recsKey changes)
  useEffect(() => {
    let cancelled = false
    getTodayRecommendations()
      .then((items) => { if (!cancelled) setRecommendations(items) })
      .catch(() => {})
      .finally(() => { if (!cancelled) setRecsLoaded(true) })
    return () => { cancelled = true }
  }, [recsKey])

  useEffect(() => {
    let cancelled = false
    setLoading(true)

    listVideos({
      page,
      page_size: pageSize,
      sort_by: sortBy,
      sort_order: sortOrder,
      q: query || undefined,
      tag_ids: tagIdsStr || undefined,
    })
      .then((res) => {
        if (cancelled) return
        setVideos(res.data)
        setTotal(res.total)
      })
      .catch(() => {
        if (cancelled) return
        setVideos([])
        setTotal(0)
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })

    return () => {
      cancelled = true
    }
  }, [page, pageSize, sortBy, sortOrder, query, tagIdsStr])

  return (
    <div className="min-h-screen bg-gray-950 flex flex-col">
      <Header searchQuery={searchInput} onSearch={handleSearch} onLogoClick={() => setRecsKey((k) => k + 1)} />

      <div className="flex flex-1 overflow-hidden">
        <TagSidebar selectedTagIds={selectedTagIds} onTagsChange={handleTagsChange} />

        <main className="flex-1 p-4 overflow-y-auto">
          {/* Today's recommendations */}
          {recsLoaded && recommendations.length > 0 && (
            <div className="mb-6">
              <h2 className="text-lg font-semibold text-white mb-3">
                {recommendations[0].is_fallback ? '為你推薦' : '今日推薦'}
              </h2>
              <div className="flex gap-3 overflow-x-auto pb-2">
                {recommendations.map((rec) => (
                  <Link
                    key={rec.id}
                    to={`/videos/${rec.video_id}`}
                    className="shrink-0 w-44 group bg-gray-900 rounded-lg overflow-hidden hover:ring-2 hover:ring-indigo-500 transition-all"
                  >
                    <div className="aspect-video bg-gray-800 relative">
                      {rec.thumbnail_url ? (
                        <img src={rec.thumbnail_url} alt={rec.title} className="w-full h-full object-cover" loading="lazy" />
                      ) : (
                        <div className="w-full h-full flex items-center justify-center text-gray-600">
                          <svg className="w-8 h-8" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M15.75 10.5l4.72-4.72a.75.75 0 011.28.53v11.38a.75.75 0 01-1.28.53l-4.72-4.72M4.5 18.75h9a2.25 2.25 0 002.25-2.25v-9a2.25 2.25 0 00-2.25-2.25h-9A2.25 2.25 0 002.25 7.5v9a2.25 2.25 0 002.25 2.25z" />
                          </svg>
                        </div>
                      )}
                      <span className="absolute bottom-1 right-1 bg-black/80 text-white text-xs px-1.5 py-0.5 rounded">
                        {formatDuration(rec.duration_seconds)}
                      </span>
                    </div>
                    <div className="p-2">
                      <h3 className="text-xs text-white font-medium line-clamp-2 group-hover:text-indigo-400 transition-colors">
                        {rec.title}
                      </h3>
                    </div>
                  </Link>
                ))}
              </div>
            </div>
          )}

          {/* Sort controls */}
          <div className="flex items-center justify-between mb-4">
            <div className="text-sm text-gray-500">
              {total > 0 ? `共 ${total} 部影片` : ''}
            </div>
            <div className="flex items-center gap-2">
              <select
                value={sortBy}
                onChange={(e) => handleSortChange(e.target.value)}
                className="bg-gray-800 text-gray-300 text-sm rounded px-2 py-1 outline-none"
              >
                {SORT_OPTIONS.map((opt) => (
                  <option key={opt.value} value={opt.value}>
                    {opt.label}
                  </option>
                ))}
              </select>
              <button
                onClick={handleSortOrderToggle}
                className="bg-gray-800 text-gray-400 hover:text-white text-sm rounded px-2 py-1 transition-colors"
                title={sortOrder === 'asc' ? '升序' : '降序'}
              >
                {sortOrder === 'asc' ? '↑' : '↓'}
              </button>
            </div>
          </div>

          {/* Video grid */}
          {loading ? (
            <div className="flex items-center justify-center py-20 text-gray-500">
              載入中...
            </div>
          ) : videos.length === 0 ? (
            <div className="flex items-center justify-center py-20 text-gray-500">
              {query || tagIdsStr ? '沒有符合條件的影片' : '尚無影片'}
            </div>
          ) : (
            <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-4">
              {videos.map((video) => (
                <VideoCard key={video.id} video={video} />
              ))}
            </div>
          )}

          <Pagination page={page} totalPages={totalPages} onPageChange={handlePageChange} />
        </main>
      </div>
    </div>
  )
}
