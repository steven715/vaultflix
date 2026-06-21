import type { ReactNode } from 'react'
import { posterGradient } from '../utils/poster'

interface PosterThumbProps {
  id: string
  title: string
  thumbnailUrl?: string
  // Tailwind aspect / rounding overrides, e.g. "aspect-video rounded-card".
  className?: string
  // Overlaid content (play button, duration badge, progress bar, etc.).
  children?: ReactNode
  // Show the title text on the gradient fallback (off for tiny thumbs).
  showFallbackTitle?: boolean
}

// PosterThumb renders a video poster: a real thumbnail when available, otherwise
// a deterministic gradient fallback derived from the video id. The inset shadow
// reproduces the "poster vignette" from the design.
export default function PosterThumb({
  id,
  title,
  thumbnailUrl,
  className = 'aspect-video rounded-card',
  children,
  showFallbackTitle = true,
}: PosterThumbProps) {
  return (
    <div className={`relative overflow-hidden bg-surface ${className}`}>
      {thumbnailUrl ? (
        <img
          src={thumbnailUrl}
          alt={title}
          loading="lazy"
          className="absolute inset-0 h-full w-full object-cover"
        />
      ) : (
        <div
          className="absolute inset-0 flex items-center justify-center p-3"
          style={{ background: posterGradient(id) }}
        >
          {showFallbackTitle && (
            <span className="line-clamp-3 text-center font-display text-sm font-bold leading-tight text-cream/95 drop-shadow">
              {title}
            </span>
          )}
        </div>
      )}
      {/* Poster vignette */}
      <div
        className="pointer-events-none absolute inset-0"
        style={{ boxShadow: 'var(--shadow-poster)' }}
      />
      {children}
    </div>
  )
}
