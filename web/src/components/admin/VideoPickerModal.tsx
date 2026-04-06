import { useState, useEffect, useRef } from 'react'
import { listVideos } from '../../api/videos'
import type { VideoWithTags } from '../../types'
import { formatDuration } from '../../utils/format'

interface VideoPickerModalProps {
  onSelect: (video: VideoWithTags) => void
  onClose: () => void
}

export default function VideoPickerModal({ onSelect, onClose }: VideoPickerModalProps) {
  const [query, setQuery] = useState('')
  const [videos, setVideos] = useState<VideoWithTags[]>([])
  const [loading, setLoading] = useState(false)
  const debounceRef = useRef<ReturnType<typeof setTimeout>>(undefined)
  const [searchTerm, setSearchTerm] = useState('')

  function handleSearch(value: string) {
    setQuery(value)
    clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(() => {
      setSearchTerm(value)
    }, 300)
  }

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    listVideos({ page: 1, page_size: 20, sort_by: 'created_at', sort_order: 'desc', q: searchTerm || undefined })
      .then((res) => { if (!cancelled) setVideos(res.data) })
      .catch(() => { if (!cancelled) setVideos([]) })
      .finally(() => { if (!cancelled) setLoading(false) })
    return () => { cancelled = true }
  }, [searchTerm])

  return (
    <div className="fixed inset-0 bg-black/60 flex items-center justify-center z-50" onClick={onClose}>
      <div className="bg-gray-900 rounded-lg p-6 w-full max-w-lg max-h-[80vh] flex flex-col" onClick={(e) => e.stopPropagation()}>
        <h2 className="text-lg font-semibold text-white mb-4">選擇影片</h2>
        <input
          value={query}
          onChange={(e) => handleSearch(e.target.value)}
          placeholder="搜尋影片..."
          className="w-full bg-gray-800 text-white text-sm rounded px-3 py-2 outline-none focus:ring-2 focus:ring-indigo-500 mb-4"
          autoFocus
        />
        <div className="flex-1 overflow-y-auto space-y-1">
          {loading ? (
            <div className="text-gray-500 text-center py-8">搜尋中...</div>
          ) : videos.length === 0 ? (
            <div className="text-gray-500 text-center py-8">沒有找到影片</div>
          ) : (
            videos.map((video) => (
              <button
                key={video.id}
                onClick={() => onSelect(video)}
                className="w-full flex items-center gap-3 p-2 rounded hover:bg-gray-800 transition-colors text-left"
              >
                <div className="w-16 aspect-video bg-gray-800 rounded overflow-hidden shrink-0">
                  {video.thumbnail_url ? (
                    <img src={video.thumbnail_url} alt="" className="w-full h-full object-cover" loading="lazy" />
                  ) : (
                    <div className="w-full h-full flex items-center justify-center text-gray-600">
                      <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M15.75 10.5l4.72-4.72a.75.75 0 011.28.53v11.38a.75.75 0 01-1.28.53l-4.72-4.72M4.5 18.75h9a2.25 2.25 0 002.25-2.25v-9a2.25 2.25 0 00-2.25-2.25h-9A2.25 2.25 0 002.25 7.5v9a2.25 2.25 0 002.25 2.25z" />
                      </svg>
                    </div>
                  )}
                </div>
                <div className="min-w-0 flex-1">
                  <div className="text-sm text-white truncate">{video.title}</div>
                  <div className="text-xs text-gray-500">{formatDuration(video.duration_seconds)}</div>
                </div>
              </button>
            ))
          )}
        </div>
        <div className="flex justify-end mt-4">
          <button onClick={onClose} className="text-sm text-gray-400 hover:text-white px-3 py-1.5 rounded">取消</button>
        </div>
      </div>
    </div>
  )
}
