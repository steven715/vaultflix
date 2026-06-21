import { Link } from 'react-router-dom'
import type { VideoWithTags } from '../types'
import { formatDuration } from '../utils/format'
import PosterThumb from './PosterThumb'

// UpNextList is the player page's right-hand "接著看" column: a vertical list of
// videos. Each row navigates to that video (the player re-fetches by :id).
export default function UpNextList({ items }: { items: VideoWithTags[] }) {
  if (items.length === 0) return null
  return (
    <div className="space-y-1">
      {items.map((v) => (
        <Link
          key={v.id}
          to={`/videos/${v.id}`}
          className="group flex gap-3 rounded-card p-2 transition-colors hover:bg-surface"
        >
          <PosterThumb
            id={v.id}
            title={v.title}
            thumbnailUrl={v.thumbnail_url}
            showFallbackTitle={false}
            className="aspect-video w-[140px] shrink-0 rounded-lg"
          >
            <span className="absolute bottom-1 right-1 rounded bg-black/70 px-1 py-0.5 font-mono text-[10px] text-cream">
              {formatDuration(v.duration_seconds)}
            </span>
          </PosterThumb>
          <div className="min-w-0 flex-1 py-0.5">
            <h4 className="line-clamp-2 text-sm font-medium leading-snug text-cream transition-colors group-hover:text-accent">
              {v.title}
            </h4>
            <div className="mt-1 font-mono text-[11px] text-muted">
              <span className="text-accent">{v.resolution}</span>
              {v.tags[0] && <span className="text-faint"> · {v.tags[0].name}</span>}
            </div>
          </div>
        </Link>
      ))}
    </div>
  )
}
