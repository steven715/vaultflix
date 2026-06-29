import { useState, useEffect, useRef } from 'react'
import { listVideos } from '../../api/videos'
import type { VideoWithTags } from '../../types'
import ErrorBanner from '../ErrorBanner'
import { formatDuration } from '../../utils/format'
import PosterThumb from './PosterThumb'

interface VideoPickerModalProps {
  onSelect: (video: VideoWithTags) => void
  onClose: () => void
}

export default function VideoPickerModal({ onSelect, onClose }: VideoPickerModalProps) {
  const [query, setQuery] = useState('')
  const [videos, setVideos] = useState<VideoWithTags[]>([])
  const [loading, setLoading] = useState(false)
  const [loadError, setLoadError] = useState(false)
  const [reloadKey, setReloadKey] = useState(0)
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
    const load = async () => {
      setLoading(true)
      setLoadError(false)
      try {
        const res = await listVideos({ page: 1, page_size: 20, sort_by: 'created_at', sort_order: 'desc', q: searchTerm || undefined })
        if (!cancelled) setVideos(res.data)
      } catch {
        if (!cancelled) { setVideos([]); setLoadError(true) }
      } finally {
        if (!cancelled) setLoading(false)
      }
    }
    load()
    return () => { cancelled = true }
  }, [searchTerm, reloadKey])

  return (
    <div
      className="fixed inset-0 flex items-center justify-center z-50"
      style={{ background: 'rgba(8,6,5,0.72)', backdropFilter: 'blur(3px)' }}
      onClick={onClose}
    >
      <div className="bg-surface border border-border rounded-lg p-6 w-full max-w-lg max-h-[80vh] flex flex-col" onClick={(e) => e.stopPropagation()}>
        <h2 className="text-lg font-semibold font-display tracking-tight text-cream mb-4">選擇影片</h2>
        <input
          value={query}
          onChange={(e) => handleSearch(e.target.value)}
          placeholder="搜尋影片..."
          className="w-full bg-surface-2 text-cream text-sm rounded-btn px-3 py-2 outline-none focus:ring-2 focus:ring-accent mb-4"
          autoFocus
        />
        <div className="flex-1 overflow-y-auto space-y-1">
          {loading ? (
            <div className="text-faint text-center py-8">搜尋中...</div>
          ) : loadError ? (
            <ErrorBanner
              message="無法載入影片，請確認服務是否正常運作"
              onRetry={() => setReloadKey((k) => k + 1)}
            />
          ) : videos.length === 0 ? (
            <div className="text-faint text-center py-8">沒有找到影片</div>
          ) : (
            videos.map((video) => (
              <button
                key={video.id}
                onClick={() => onSelect(video)}
                className="w-full flex items-center gap-3 p-2 rounded-btn hover:bg-surface-2 transition-colors text-left"
              >
                <PosterThumb id={video.id} src={video.thumbnail_url} className="w-16 shrink-0" />
                <div className="min-w-0 flex-1">
                  <div className="text-sm text-cream truncate">{video.title}</div>
                  <div className="text-xs text-faint font-mono">{formatDuration(video.duration_seconds)}</div>
                </div>
              </button>
            ))
          )}
        </div>
        <div className="flex justify-end mt-4">
          <button onClick={onClose} className="text-sm text-muted hover:text-cream px-3 py-1.5 rounded-btn">取消</button>
        </div>
      </div>
    </div>
  )
}
