import { useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { useMatchMedia } from '../hooks/useMatchMedia'
import type { RecommendationItem } from '../types'
import { formatDuration } from '../utils/format'
import PosterThumb from './PosterThumb'
import { PlayIcon } from './icons'

interface RecommendationCardProps {
  rec: RecommendationItem
}

const HOVER_DELAY_MS = 300

// RecommendationCard is the smaller "為你推薦 / 今日推薦" rail card.
export default function RecommendationCard({ rec }: RecommendationCardProps) {
  const supportsHover = useMatchMedia('(hover: hover)')
  const [showPreview, setShowPreview] = useState(false)
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  useEffect(() => {
    return () => {
      if (timerRef.current) clearTimeout(timerRef.current)
    }
  }, [])

  const canPreview = supportsHover && Boolean(rec.preview_url)

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
      to={`/videos/${rec.video_id}`}
      className="group block w-[180px] shrink-0 transition-transform duration-200 hover:-translate-y-[5px] sm:w-[244px]"
      onMouseEnter={onMouseEnter}
      onMouseLeave={onMouseLeave}
    >
      <PosterThumb
        id={rec.video_id}
        title={rec.title}
        thumbnailUrl={rec.thumbnail_url}
        className="aspect-video rounded-card"
      >
        {showPreview && rec.preview_url && (
          <video
            src={rec.preview_url}
            autoPlay
            muted
            loop
            playsInline
            preload="metadata"
            onError={() => setShowPreview(false)}
            className="absolute inset-0 h-full w-full object-cover"
          />
        )}
        <div className="absolute inset-0 flex items-center justify-center opacity-0 transition-opacity duration-200 group-hover:opacity-100">
          <span className="flex h-11 w-11 items-center justify-center rounded-full border border-white/20 bg-black/40 backdrop-blur-sm">
            <PlayIcon className="ml-0.5 h-4.5 w-4.5 text-cream" />
          </span>
        </div>
        <span className="absolute bottom-2 right-2 rounded-md bg-black/70 px-1.5 py-0.5 font-mono text-[11px] text-cream">
          {formatDuration(rec.duration_seconds)}
        </span>
      </PosterThumb>
      <h3 className="mt-2 line-clamp-2 text-sm font-semibold leading-snug text-cream transition-colors group-hover:text-accent">
        {rec.title}
      </h3>
    </Link>
  )
}
