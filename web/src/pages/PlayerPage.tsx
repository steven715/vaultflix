import { useState, useEffect, useRef, useCallback } from 'react'
import { useParams, Link, useNavigate } from 'react-router-dom'
import Hls from 'hls.js'
import { getVideo, getStreamToken, listVideos } from '../api/videos'
import { saveProgress } from '../api/watchHistory'
import { addFavorite, removeFavorite } from '../api/favorites'
import { postHeartbeat } from '../api/watchSession'
import type { VideoDetail, VideoWithTags } from '../types'
import { formatDuration, formatFileSize, formatDate } from '../utils/format'
import { useToast } from '../contexts/ToastContext'
import AppShell, { Container } from '../components/AppShell'
import UpNextList from '../components/UpNextList'
import { ChevronLeft, HeartIcon, HeartFilled, CheckIcon, ShareIcon } from '../components/icons'
import { clampDelta } from '../lib/heartbeat'

const PROGRESS_THROTTLE_MS = 10_000
const HEARTBEAT_INTERVAL_MS = 15_000

export default function PlayerPage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const toast = useToast()
  const [video, setVideo] = useState<VideoDetail | null>(null)
  const [streamToken, setStreamToken] = useState('')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [favorited, setFavorited] = useState(false)
  const [upNext, setUpNext] = useState<VideoWithTags[]>([])
  const videoRef = useRef<HTMLVideoElement>(null)
  const retryCountRef = useRef(0)
  // Playback position to restore after a stream-token refresh reload.
  const pendingSeekRef = useRef<number | null>(null)

  // Progress reporting refs (no state to avoid re-renders)
  const lastReportTimeRef = useRef(0)
  const lastReportSecondsRef = useRef(-1)
  const videoIDRef = useRef<string>('')

  // Heartbeat (accumulated real watch time) — session regenerates per open.
  const sessionIdRef = useRef<string>('')
  const lastSampleSecondsRef = useRef(0) // last currentTime we measured a delta from
  const pendingDeltaRef = useRef(0) // play-seconds accumulated since last flush

  // Flush the accumulated heartbeat delta. Defined before the effects below
  // (rather than near handleTimeUpdate) because the fetchVideo effect's
  // dependency array references it directly, and a `const`/useCallback
  // binding — unlike a hoisted `function` declaration — must already be
  // initialized at that point in source order. Reads only refs, so this is
  // a stable useCallback (identity never changes across renders).
  const flushHeartbeat = useCallback((useBeacon = false) => {
    const vid = videoIDRef.current
    const sid = sessionIdRef.current
    const delta = pendingDeltaRef.current
    if (!vid || !sid || delta <= 0) return
    pendingDeltaRef.current = 0
    const el = videoRef.current
    const position = el ? Math.floor(el.currentTime) : 0
    const payload = { session_id: sid, video_id: vid, played_delta: delta, position_seconds: position }
    if (useBeacon) {
      const token = localStorage.getItem('token')
      fetch('/api/watch-sessions/heartbeat', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', ...(token ? { Authorization: `Bearer ${token}` } : {}) },
        body: JSON.stringify(payload),
        keepalive: true,
      }).catch(() => {})
      return
    }
    postHeartbeat(payload).catch((err) => console.warn('failed to send heartbeat', err))
  }, [])

  useEffect(() => {
    let cancelled = false
    if (!id) return

    // Scroll to top when switching videos (e.g. via the up-next list).
    window.scrollTo({ top: 0 })

    const fetchVideo = async () => {
      try {
        const data = await getVideo(id)
        if (cancelled) return
        setVideo(data)
        setFavorited(data.is_favorited)
        setError('')
        retryCountRef.current = 0
        videoIDRef.current = data.id
        sessionIdRef.current = crypto.randomUUID()
        lastSampleSecondsRef.current = 0
        pendingDeltaRef.current = 0

        const { token } = await getStreamToken(id)
        if (!cancelled) setStreamToken(token)
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
      flushHeartbeat(true)
    }
  }, [id, flushHeartbeat])

  // Up-next column: a handful of other videos, excluding the current one.
  useEffect(() => {
    if (!id) return
    let cancelled = false
    listVideos({ page: 1, page_size: 12, sort_by: 'created_at', sort_order: 'desc' })
      .then((res) => {
        if (cancelled) return
        setUpNext(res.data.filter((v) => v.id !== id).slice(0, 8))
      })
      .catch((err) => console.warn('failed to load up-next', err))
    return () => {
      cancelled = true
    }
  }, [id])

  // Reload the media element whenever the stream token changes (initial load
  // and post-expiry refresh). Only for direct mode; remux is handled below.
  useEffect(() => {
    if (video?.play_mode === 'direct' && streamToken && videoRef.current) {
      videoRef.current.load()
    }
  }, [video, streamToken])

  // remux 影片用 hls.js 播放即時 HLS；direct 影片維持原生 src。
  useEffect(() => {
    const videoID = video?.id
    const playMode = video?.play_mode
    if (!videoID || playMode !== 'remux' || !streamToken || !videoRef.current) return
    const el = videoRef.current
    const url = `/api/videos/${videoID}/hls/index.m3u8?token=${streamToken}`

    // Safari 原生支援 HLS。
    if (el.canPlayType('application/vnd.apple.mpegurl')) {
      el.src = url
      return () => {
        el.removeAttribute('src')
        el.load()
      }
    }
    if (!Hls.isSupported()) {
      setError('此瀏覽器不支援串流播放')
      return
    }
    const hls = new Hls()
    hls.loadSource(url)
    hls.attachMedia(el)
    return () => {
      hls.destroy()
    }
  }, [video?.id, video?.play_mode, streamToken])

  // Keyboard shortcuts: space toggles play/pause, arrows seek ±5s.
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      const el = videoRef.current
      if (!el) return
      const target = e.target
      if (target instanceof HTMLInputElement || target instanceof HTMLTextAreaElement) return
      if (e.key === ' ') {
        e.preventDefault()
        if (el.paused) el.play()
        else el.pause()
      } else if (e.key === 'ArrowRight') {
        el.currentTime = Math.min(el.duration || Infinity, el.currentTime + 5)
      } else if (e.key === 'ArrowLeft') {
        el.currentTime = Math.max(0, el.currentTime - 5)
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

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
    }).catch((err) => {
      console.warn('failed to send progress beacon', err)
    })
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
    // Accumulate real play time for the heartbeat.
    pendingDeltaRef.current += clampDelta(lastSampleSecondsRef.current, el.currentTime)
    lastSampleSecondsRef.current = el.currentTime
  }

  // Flush the accumulated heartbeat delta on a fixed cadence.
  useEffect(() => {
    const timer = setInterval(() => flushHeartbeat(false), HEARTBEAT_INTERVAL_MS)
    return () => clearInterval(timer)
  }, [flushHeartbeat])

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

  // Resume playback from watch_progress + restore volume
  function handleLoadedMetadata() {
    if (!video || !videoRef.current) return
    // A successful (re)load means recovery — reset the error-retry budget so a
    // later, unrelated transient error still gets its one retry instead of
    // failing outright.
    retryCountRef.current = 0
    const savedVolume = localStorage.getItem('vaultflix-volume')
    if (savedVolume !== null) {
      videoRef.current.volume = parseFloat(savedVolume)
    }
    if (pendingSeekRef.current != null) {
      // Restoring position after a token-refresh reload — not a fresh open.
      videoRef.current.currentTime = pendingSeekRef.current
      pendingSeekRef.current = null
      lastSampleSecondsRef.current = videoRef.current.currentTime
    } else if (video.watch_progress > 0) {
      videoRef.current.currentTime = video.watch_progress
      lastSampleSecondsRef.current = videoRef.current.currentTime
      toast.info(`從 ${formatDuration(video.watch_progress)} 繼續播放`)
    }
  }

  function handleVolumeChange() {
    if (!videoRef.current) return
    localStorage.setItem('vaultflix-volume', String(videoRef.current.volume))
  }

  // Handle stream-token expiry: refresh the scoped token on video error
  // (max 1 retry per error episode; the budget resets on a successful load).
  function handleVideoError() {
    if (!video || retryCountRef.current >= 1) {
      if (retryCountRef.current >= 1) {
        setError('影片載入失敗')
      }
      return
    }
    retryCountRef.current += 1
    // Preserve the current position; the streamToken effect reloads the src.
    pendingSeekRef.current = videoRef.current?.currentTime ?? null
    getStreamToken(video.id)
      .then(({ token }) => setStreamToken(token))
      .catch(() => {
        setError('影片串流憑證更新失敗')
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
      toast.error(prev ? '取消收藏失敗' : '加入收藏失敗')
    } finally {
      favoriteInFlightRef.current = false
    }
  }

  function handleMarkWatched() {
    if (!video) return
    saveProgress(video.id, video.duration_seconds)
      .then(() => toast.success('已標記為看完'))
      .catch(() => toast.error('標記失敗'))
  }

  async function handleShare() {
    try {
      await navigator.clipboard.writeText(window.location.href)
      toast.success('連結已複製')
    } catch {
      toast.error('複製連結失敗')
    }
  }

  if (loading) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-bg text-muted">載入中…</div>
    )
  }

  if (error || !video) {
    return (
      <div className="flex min-h-screen flex-col items-center justify-center gap-4 bg-bg">
        <div className="text-muted">{error || '影片不存在'}</div>
        <Link to="/" className="text-sm text-accent hover:underline">
          返回片庫
        </Link>
      </div>
    )
  }

  return (
    <AppShell showTabBar={false}>
      <Container className="py-6">
        <button
          onClick={() => navigate('/')}
          className="mb-5 flex items-center gap-1.5 text-sm text-muted transition-colors hover:text-cream"
        >
          <ChevronLeft className="h-4 w-4" />
          返回片庫
        </button>

        <div className="flex flex-col gap-8 lg:flex-row">
          {/* Player + info */}
          <div className="min-w-0 flex-1">
            {video.play_mode === 'transcode' ? (
              <div className="flex aspect-video w-full items-center justify-center rounded-lg bg-surface text-center text-sm text-muted">
                <div className="px-6">
                  此影片格式（{video.video_codec || '未知編碼'}）尚未支援線上播放，
                  將於後續版本（Phase 2 轉碼）支援。
                </div>
              </div>
            ) : (
              <div className="relative overflow-hidden rounded-lg bg-black">
                <video
                  ref={videoRef}
                  controls
                  preload="metadata"
                  src={
                    video.play_mode === 'direct' && streamToken
                      ? `${video.stream_url}?token=${streamToken}`
                      : undefined
                  }
                  className="aspect-video w-full"
                  onError={handleVideoError}
                  onTimeUpdate={handleTimeUpdate}
                  onPause={handlePause}
                  onLoadedMetadata={handleLoadedMetadata}
                  onVolumeChange={handleVolumeChange}
                />
              </div>
            )}

            <div className="mt-5">
              <div className="flex flex-wrap items-start justify-between gap-3">
                <h1 className="font-display text-2xl font-bold tracking-tight text-cream md:text-[32px]">
                  {video.title}
                </h1>
                <div className="flex shrink-0 items-center gap-2">
                  <ActionButton
                    active={favorited}
                    onClick={handleFavoriteToggle}
                    icon={favorited ? <HeartFilled className="h-4 w-4" /> : <HeartIcon className="h-4 w-4" />}
                    label={favorited ? '已收藏' : '收藏'}
                  />
                  <ActionButton
                    onClick={handleMarkWatched}
                    icon={<CheckIcon className="h-4 w-4" />}
                    label="標記已看"
                  />
                  <ActionButton
                    onClick={handleShare}
                    icon={<ShareIcon className="h-4 w-4" />}
                    label="分享"
                  />
                </div>
              </div>

              <div className="mt-3 flex flex-wrap items-center gap-x-3 gap-y-1 font-mono text-xs text-muted">
                <span className="text-accent">{video.resolution}</span>
                <span className="text-faint">·</span>
                <span>{formatDuration(video.duration_seconds)}</span>
                <span className="text-faint">·</span>
                <span>{formatFileSize(video.file_size_bytes)}</span>
                <span className="text-faint">·</span>
                <span>{formatDate(video.created_at)}</span>
              </div>

              {video.description && (
                <p className="mt-4 max-w-[760px] text-sm leading-relaxed text-muted">
                  {video.description}
                </p>
              )}

              {video.tags.length > 0 && (
                <div className="mt-4 flex flex-wrap gap-2">
                  {video.tags.map((tag) => (
                    <span
                      key={tag.id}
                      className="rounded-pill bg-surface px-3 py-1 text-xs text-muted"
                    >
                      {tag.name}
                    </span>
                  ))}
                </div>
              )}
            </div>
          </div>

          {/* Up next */}
          <aside className="w-full shrink-0 lg:w-[380px]">
            <h2 className="mb-3 font-display text-lg font-bold text-cream">接著看</h2>
            <UpNextList items={upNext} />
          </aside>
        </div>
      </Container>
    </AppShell>
  )
}

function ActionButton({
  icon,
  label,
  onClick,
  active = false,
}: {
  icon: React.ReactNode
  label: string
  onClick: () => void
  active?: boolean
}) {
  return (
    <button
      onClick={onClick}
      className={`flex items-center gap-1.5 rounded-pill border px-3.5 py-2 text-sm font-medium transition-colors active:scale-95 ${
        active
          ? 'border-fav/40 bg-fav/10 text-fav'
          : 'border-border bg-surface text-muted hover:text-cream'
      }`}
    >
      {icon}
      <span className="hidden sm:inline">{label}</span>
    </button>
  )
}
