import { useState, useEffect, useRef, useCallback } from 'react'
import { useParams, Link } from 'react-router-dom'
import { getVideo } from '../api/videos'
import { saveProgress } from '../api/watchHistory'
import { addFavorite, removeFavorite } from '../api/favorites'
import type { VideoDetail } from '../types'
import { formatDuration, formatFileSize, formatDate } from '../utils/format'
import { useAuth } from '../contexts/AuthContext'

const PROGRESS_THROTTLE_MS = 10_000

export default function PlayerPage() {
  const { id } = useParams<{ id: string }>()
  const { token } = useAuth()
  const [video, setVideo] = useState<VideoDetail | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [favorited, setFavorited] = useState(false)
  const [toast, setToast] = useState('')
  const videoRef = useRef<HTMLVideoElement>(null)
  const retryCountRef = useRef(0)

  // Progress reporting refs (no state to avoid re-renders)
  const lastReportTimeRef = useRef(0)
  const lastReportSecondsRef = useRef(-1)
  const videoIDRef = useRef<string>('')

  useEffect(() => {
    let cancelled = false
    if (!id) return

    const fetchVideo = async () => {
      try {
        const data = await getVideo(id)
        if (!cancelled) {
          setVideo(data)
          setFavorited(data.is_favorited)
          setError('')
          retryCountRef.current = 0
          videoIDRef.current = data.id
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
    return () => {
      cancelled = true
      // Send final progress on unmount
      sendProgressBeacon()
    }
  }, [id])

  // Send progress via sendBeacon for unmount/page leave
  function sendProgressBeacon() {
    const vid = videoIDRef.current
    const el = videoRef.current
    if (!vid || !el || el.currentTime < 1) return

    const seconds = Math.floor(el.currentTime)
    if (seconds === lastReportSecondsRef.current) return

    const token = localStorage.getItem('token')
    fetch('/api/watch-history', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
      },
      body: JSON.stringify({ video_id: vid, progress_seconds: seconds }),
      keepalive: true,
    }).catch(() => {})
  }

  // Throttled progress reporter
  const reportProgress = useCallback((currentTime: number) => {
    const now = Date.now()
    const seconds = Math.floor(currentTime)
    if (
      seconds === lastReportSecondsRef.current ||
      now - lastReportTimeRef.current < PROGRESS_THROTTLE_MS
    ) {
      return
    }

    lastReportTimeRef.current = now
    lastReportSecondsRef.current = seconds

    saveProgress(videoIDRef.current, seconds).catch((err) => {
      console.warn('failed to report progress', err)
    })
  }, [])

  function handleTimeUpdate() {
    const el = videoRef.current
    if (!el) return
    reportProgress(el.currentTime)
  }

  function handlePause() {
    const el = videoRef.current
    if (!el || el.currentTime < 1) return
    const seconds = Math.floor(el.currentTime)
    if (seconds === lastReportSecondsRef.current) return

    lastReportTimeRef.current = Date.now()
    lastReportSecondsRef.current = seconds

    saveProgress(videoIDRef.current, seconds).catch((err) => {
      console.warn('failed to report progress on pause', err)
    })
  }

  // Resume playback from watch_progress
  function handleLoadedMetadata() {
    if (!video || !videoRef.current) return
    if (video.watch_progress > 0) {
      videoRef.current.currentTime = video.watch_progress
      setToast(`從 ${formatDuration(video.watch_progress)} 繼續播放`)
      setTimeout(() => setToast(''), 3000)
    }
  }

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

  // Favorite toggle with optimistic UI
  const favoriteInFlightRef = useRef(false)
  async function handleFavoriteToggle() {
    if (!video || favoriteInFlightRef.current) return
    favoriteInFlightRef.current = true

    const prev = favorited
    setFavorited(!prev)

    try {
      if (prev) {
        await removeFavorite(video.id)
      } else {
        await addFavorite(video.id)
      }
    } catch {
      setFavorited(prev)
      setToast(prev ? '取消收藏失敗' : '加入收藏失敗')
      setTimeout(() => setToast(''), 3000)
    } finally {
      favoriteInFlightRef.current = false
    }
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
        <div className="bg-black rounded-lg overflow-hidden relative">
          <video
            ref={videoRef}
            controls
            preload="metadata"
            src={token ? `${video.stream_url}?token=${token}` : undefined}
            className="w-full"
            onError={handleVideoError}
            onTimeUpdate={handleTimeUpdate}
            onPause={handlePause}
            onLoadedMetadata={handleLoadedMetadata}
          />
          {/* Toast */}
          {toast && (
            <div className="absolute top-4 left-1/2 -translate-x-1/2 bg-black/80 text-white text-sm px-4 py-2 rounded-lg pointer-events-none">
              {toast}
            </div>
          )}
        </div>

        {/* Video info */}
        <div className="mt-4 space-y-3">
          <div className="flex items-start justify-between gap-3">
            <h1 className="text-xl font-semibold text-white">{video.title}</h1>

            {/* Favorite button */}
            <button
              onClick={handleFavoriteToggle}
              className="shrink-0 p-2 rounded-full hover:bg-gray-800 transition-all active:scale-90"
              title={favorited ? '取消收藏' : '加入收藏'}
            >
              <svg
                className={`w-6 h-6 transition-colors duration-200 ${
                  favorited ? 'text-red-500 fill-red-500' : 'text-gray-400 fill-none'
                }`}
                viewBox="0 0 24 24"
                stroke="currentColor"
                strokeWidth={2}
              >
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  d="M21 8.25c0-2.485-2.099-4.5-4.688-4.5-1.935 0-3.597 1.126-4.312 2.733-.715-1.607-2.377-2.733-4.313-2.733C5.1 3.75 3 5.765 3 8.25c0 7.22 9 12 9 12s9-4.78 9-12z"
                />
              </svg>
            </button>
          </div>

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
