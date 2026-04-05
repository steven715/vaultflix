import { useState, useEffect, useCallback, useRef } from 'react'
import { useSearchParams } from 'react-router-dom'
import { listVideos } from '../api/videos'
import type { VideoWithTags } from '../types'
import Header from '../components/Header'
import TagSidebar from '../components/TagSidebar'
import VideoCard from '../components/VideoCard'
import Pagination from '../components/Pagination'

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
      <Header searchQuery={searchInput} onSearch={handleSearch} />

      <div className="flex flex-1 overflow-hidden">
        <TagSidebar selectedTagIds={selectedTagIds} onTagsChange={handleTagsChange} />

        <main className="flex-1 p-4 overflow-y-auto">
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
