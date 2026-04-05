import { useState, useEffect, useRef } from 'react'
import { useParams, Link } from 'react-router-dom'
import { getVideo } from '../api/videos'
import type { VideoDetail } from '../types'

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

function formatDate(dateStr: string): string {
  return new Date(dateStr).toLocaleDateString('zh-TW', {
    year: 'numeric',
    month: 'long',
    day: 'numeric',
  })
}

export default function PlayerPage() {
  const { id } = useParams<{ id: string }>()
  const [video, setVideo] = useState<VideoDetail | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const videoRef = useRef<HTMLVideoElement>(null)
  const retryCountRef = useRef(0)

  useEffect(() => {
    let cancelled = false
    if (!id) return

    const fetchVideo = async () => {
      try {
        const data = await getVideo(id)
        if (!cancelled) {
          setVideo(data)
          setError('')
          retryCountRef.current = 0
        }
      } catch {
        if (!cancelled) {
          setError('無法載入影片')
        }
      } finally {
        if (!cancelled) {
          setLoading(false)
        }
      }
    }

    fetchVideo()
    return () => { cancelled = true }
  }, [id])

  // Handle presigned URL expiry: re-fetch on video error (max 1 retry)
  function handleVideoError() {
    if (!video || retryCountRef.current >= 1) {
      if (retryCountRef.current >= 1) {
        setError('影片載入失敗')
      }
      return
    }
    retryCountRef.current += 1
    getVideo(video.id)
      .then((data) => {
        setVideo(data)
        if (videoRef.current) {
          videoRef.current.load()
        }
      })
      .catch(() => {
        setError('影片 URL 已過期，重新取得失敗')
      })
  }

  if (loading) {
    return (
      <div className="min-h-screen bg-gray-950 flex items-center justify-center text-gray-500">
        載入中...
      </div>
    )
  }

  if (error || !video) {
    return (
      <div className="min-h-screen bg-gray-950 flex flex-col items-center justify-center gap-4">
        <div className="text-gray-400">{error || '影片不存在'}</div>
        <Link to="/" className="text-indigo-400 hover:text-indigo-300 text-sm">
          返回瀏覽頁
        </Link>
      </div>
    )
  }

  return (
    <div className="min-h-screen bg-gray-950">
      {/* Back button */}
      <div className="px-4 py-3">
        <Link to="/" className="text-sm text-gray-400 hover:text-white transition-colors">
          ← 返回
        </Link>
      </div>

      {/* Video player */}
      <div className="max-w-5xl mx-auto px-4">
        <div className="bg-black rounded-lg overflow-hidden">
          <video
            ref={videoRef}
            controls
            preload="metadata"
            src={video.stream_url}
            className="w-full"
            onError={handleVideoError}
          />
        </div>

        {/* Video info */}
        <div className="mt-4 space-y-3">
          <h1 className="text-xl font-semibold text-white">{video.title}</h1>

          {video.description && (
            <p className="text-sm text-gray-400">{video.description}</p>
          )}

          <div className="flex flex-wrap gap-x-4 gap-y-1 text-sm text-gray-500">
            <span>{formatDuration(video.duration_seconds)}</span>
            <span>{video.resolution}</span>
            <span>{formatFileSize(video.file_size_bytes)}</span>
            <span>{video.mime_type}</span>
            <span>{formatDate(video.created_at)}</span>
          </div>

          {video.tags.length > 0 && (
            <div className="flex flex-wrap gap-2">
              {video.tags.map((tag) => (
                <span
                  key={tag.id}
                  className="text-xs bg-gray-800 text-gray-400 px-2 py-1 rounded"
                >
                  {tag.name}
                </span>
              ))}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
