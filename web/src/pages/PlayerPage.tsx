import { useState, useEffect, useRef, useCallback } from 'react'
import { useParams, Link, useNavigate, useLocation, useSearchParams } from 'react-router-dom'
import { getVideo, listVideos } from '../api/videos'
import { saveProgress } from '../api/watchHistory'
import { addFavorite, removeFavorite } from '../api/favorites'
import { getTodayRecommendations } from '../api/recommendations'
import type { VideoDetail, VideoWithTags, RecommendationItem } from '../types'
import { formatDuration, formatFileSize, formatDate } from '../utils/format'
import { useToast } from '../contexts/ToastContext'
import AppShell, { Container } from '../components/AppShell'
import UpNextList from '../components/UpNextList'
import RecommendationList from '../components/RecommendationList'
import NetworkHud from '../components/NetworkHud'
import { ChevronLeft, HeartIcon, HeartFilled, CheckIcon, ShareIcon } from '../components/icons'
import { usePlaybackStats } from '../hooks/usePlaybackStats'
import { useStreamSource } from '../hooks/useStreamSource'
import { usePlaybackSession } from '../hooks/usePlaybackSession'
import type { PositionReason, StreamFailure } from '../lib/streamSource'

const streamFailureMessage: Record<StreamFailure, string> = {
  'media-error': '影片載入失敗',
  'stream-load-failed': '串流載入失敗',
  'preparing-timeout': '首次播放準備逾時,請稍後重試',
  'token-unavailable': '無法載入影片',
  'token-refresh-failed': '影片串流憑證更新失敗',
  unsupported: '此瀏覽器不支援串流播放',
}

export default function PlayerPage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const location = useLocation()
  const toast = useToast()
  const [video, setVideo] = useState<VideoDetail | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [favorited, setFavorited] = useState(false)
  const [upNext, setUpNext] = useState<VideoWithTags[]>([])
  const [recommendations, setRecommendations] = useState<RecommendationItem[]>([])
  const videoRef = useRef<HTMLVideoElement>(null)

  // Playback telemetry: stream inputs feed usePlaybackStats (bitrate-based
  // throughput estimate + TTFB matching), which must be mounted unconditionally
  // at the top level regardless of the loading/error early returns below.
  const avgBitrateBps =
    video && video.duration_seconds > 0
      ? (video.file_size_bytes * 8) / video.duration_seconds
      : null
  const streamPath = video
    ? video.play_mode === 'remux'
      ? `/api/videos/${video.id}/hls`
      : video.stream_url
    : null
  const { stats, getSessionSummary } = usePlaybackStats(videoRef, streamPath, avgBitrateBps)

  const [searchParams] = useSearchParams()
  const hudVisible =
    searchParams.get('hud') === '1' || localStorage.getItem('vaultflix-hud') === '1'

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
    }
  }, [id])

  // Up-next column: 5 random other videos, excluding the current one. A random
  // sample (sort_by: 'random') rather than "newest", so the list differs on each
  // video you open instead of always showing the same latest few. Fetch 6 so that
  // excluding the current one (if it lands in the sample) still fills 5 rows.
  useEffect(() => {
    if (!id) return
    let cancelled = false
    listVideos({ page: 1, page_size: 6, sort_by: 'random' })
      .then((res) => {
        if (cancelled) return
        setUpNext(res.data.filter((v) => v.id !== id).slice(0, 5))
      })
      .catch((err) => console.warn('failed to load up-next', err))
    return () => {
      cancelled = true
    }
  }, [id])

  // Today's recommendations (same source as the home page). Refetch on every
  // navigation — location.key changes even when navigating to the same :id, so
  // clicking another up-next item (or re-opening the same video) reloads these.
  useEffect(() => {
    let cancelled = false
    getTodayRecommendations(5)
      .then((items) => {
        if (!cancelled) setRecommendations(items.slice(0, 5))
      })
      .catch((err) => console.warn('failed to load recommendations', err))
    return () => {
      cancelled = true
    }
  }, [location.key])

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

  // Reporting for this Playback Session (Watch Progress, heartbeat, telemetry).
  const positionSet = usePlaybackSession({ mediaRef: videoRef, video, ready: !loading, summary: getSessionSummary })

  // The Stream Source moved the playback position itself (resume from Watch
  // Progress, or back to where a token-refresh reload interrupted): that jump is
  // not watch time.
  const handlePositionSet = useCallback(
    (seconds: number, reason: PositionReason) => {
      positionSet(seconds)
      if (reason === 'resume') toast.info(`從 ${formatDuration(seconds)} 繼續播放`)
    },
    [positionSet, toast],
  )
  const stream = useStreamSource({ mediaRef: videoRef, video, ready: !loading, onPositionSet: handlePositionSet })

  // Restore the remembered volume once metadata is in.
  function handleLoadedMetadata() {
    if (!videoRef.current) return
    const savedVolume = localStorage.getItem('vaultflix-volume')
    if (savedVolume !== null) {
      videoRef.current.volume = parseFloat(savedVolume)
    }
  }

  function handleVolumeChange() {
    if (!videoRef.current) return
    localStorage.setItem('vaultflix-volume', String(videoRef.current.volume))
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

  const streamError = stream.status === 'failed' ? streamFailureMessage[stream.reason] : ''
  if (error || streamError || !video) {
    return (
      <div className="flex min-h-screen flex-col items-center justify-center gap-4 bg-bg">
        <div className="text-muted">{error || streamError || '影片不存在'}</div>
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
                  className="aspect-video w-full"
                  onLoadedMetadata={handleLoadedMetadata}
                  onVolumeChange={handleVolumeChange}
                />
                {hudVisible && <NetworkHud stats={stats} />}
                {stream.status === 'preparing' && (
                  <div className="absolute inset-0 flex items-center justify-center bg-black/80 text-center text-sm text-muted">
                    <div className="px-6">首次播放準備中，索引建立後將自動開始…</div>
                  </div>
                )}
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

          {/* Recommendations (top) + up next (below) */}
          <aside className="w-full shrink-0 lg:w-[380px]">
            {recommendations.length > 0 && (
              <div className="mb-8">
                <h2 className="mb-3 font-display text-lg font-bold text-cream">今日推薦</h2>
                <RecommendationList items={recommendations} />
              </div>
            )}
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
