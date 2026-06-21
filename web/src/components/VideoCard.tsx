import { useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { useMatchMedia } from '../hooks/useMatchMedia'
import type { VideoWithTags } from '../types'
import { formatDuration, formatFileSize } from '../utils/format'
import PosterThumb from './PosterThumb'
import { PlayIcon, HeartFilled } from './icons'

interface VideoCardProps {
  video: VideoWithTags
  // When provided, renders a glass "unfavorite" heart button on the poster
  // (favorites page). The handler must stopPropagation to avoid navigation.
  onUnfavorite?: (videoID: string) => void
}

const HOVER_DELAY_MS = 300

// VideoCard is the 16:10 poster card used in the library grid and favorites grid.
// Hovering (on devices that support it) plays the preview clip; the poster also
// shows a center play button and a duration badge.
export default function VideoCard({ video, onUnfavorite }: VideoCardProps) {
  const supportsHover = useMatchMedia('(hover: hover)')
  const [showPreview, setShowPreview] = useState(false)
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  useEffect(() => {
    return () => {
      if (timerRef.current) clearTimeout(timerRef.current)
    }
  }, [])

  const canPreview = supportsHover && Boolean(video.preview_url)

  function onMouseEnter() {
    if (!canPreview) return
    if (timerRef.current) clearTimeout(timerRef.current)
    timerRef.current = setTimeout(() => setShowPreview(true), HOVER_DELAY_MS)
  }

  function onMouseLeave() {
    if (timerRef.current) {
      clearTimeout(timerRef.current)
      timerRef.current = null
    }
    setShowPreview(false)
  }

  return (
    <Link
      to={`/videos/${video.id}`}
      className="group block transition-transform duration-200 hover:-translate-y-[5px]"
      onMouseEnter={onMouseEnter}
      onMouseLeave={onMouseLeave}
    >
      <PosterThumb
        id={video.id}
        title={video.title}
        thumbnailUrl={video.thumbnail_url}
        className="aspect-[16/10] rounded-card"
      >
        {showPreview && video.preview_url && (
          <video
            src={video.preview_url}
            autoPlay
            muted
            loop
            playsInline
            preload="metadata"
            onError={() => setShowPreview(false)}
            className="absolute inset-0 h-full w-full object-cover"
          />
        )}

        {/* Center play affordance on hover */}
        <div className="absolute inset-0 flex items-center justify-center opacity-0 transition-opacity duration-200 group-hover:opacity-100">
          <span className="flex h-12 w-12 items-center justify-center rounded-full border border-white/20 bg-black/40 backdrop-blur-sm">
            <PlayIcon className="ml-0.5 h-5 w-5 text-cream" />
          </span>
        </div>

        {/* Duration badge */}
        <span className="absolute bottom-2 right-2 rounded-md bg-black/70 px-1.5 py-0.5 font-mono text-[11px] text-cream">
          {formatDuration(video.duration_seconds)}
        </span>

        {onUnfavorite && (
          <button
            onClick={(e) => {
              e.preventDefault()
              e.stopPropagation()
              onUnfavorite(video.id)
            }}
            title="取消收藏"
            className="absolute right-2 top-2 flex h-8 w-8 items-center justify-center rounded-full border border-white/15 bg-black/45 text-fav backdrop-blur-sm transition-transform hover:scale-110"
          >
            <HeartFilled className="h-4 w-4" />
          </button>
        )}
      </PosterThumb>

      <div className="mt-2.5">
        <h3 className="line-clamp-2 text-sm font-semibold leading-snug text-cream transition-colors group-hover:text-accent">
          {video.title}
        </h3>
        <div className="mt-1 flex items-center gap-1.5 font-mono text-[11px] text-muted">
          <span className="text-accent">{video.resolution}</span>
          <span className="text-faint">·</span>
          <span>{formatFileSize(video.file_size_bytes)}</span>
        </div>
        {video.tags.length > 0 && (
          <div className="mt-2 flex flex-wrap gap-1">
            {video.tags.slice(0, 3).map((tag) => (
              <span
                key={tag.id}
                className="rounded-pill bg-surface px-2 py-0.5 text-[11px] text-muted"
              >
                {tag.name}
              </span>
            ))}
            {video.tags.length > 3 && (
              <span className="text-[11px] text-faint">+{video.tags.length - 3}</span>
            )}
          </div>
        )}
      </div>
    </Link>
  )
}
