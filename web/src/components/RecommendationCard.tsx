import { useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { useMatchMedia } from '../hooks/useMatchMedia'
import type { RecommendationItem } from '../types'
import { formatDuration } from '../utils/format'

interface RecommendationCardProps {
  rec: RecommendationItem
}

const HOVER_DELAY_MS = 300

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

  function onPreviewError() {
    setShowPreview(false)
  }

  return (
    <Link
      to={`/videos/${rec.video_id}`}
      className="shrink-0 w-44 group bg-gray-900 rounded-lg overflow-hidden hover:ring-2 hover:ring-indigo-500 transition-all"
      onMouseEnter={onMouseEnter}
      onMouseLeave={onMouseLeave}
    >
      <div className="aspect-video bg-gray-800 relative">
        {showPreview && rec.preview_url ? (
          <video
            src={rec.preview_url}
            autoPlay
            muted
            loop
            playsInline
            preload="metadata"
            onError={onPreviewError}
            className="w-full h-full object-cover"
          />
        ) : rec.thumbnail_url ? (
          <img
            src={rec.thumbnail_url}
            alt={rec.title}
            className="w-full h-full object-cover"
            loading="lazy"
          />
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
  )
}
