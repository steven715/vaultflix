import { Link } from 'react-router-dom'
import type { RecommendationItem } from '../types'
import { formatDuration } from '../utils/format'
import PosterThumb from './PosterThumb'

// RecommendationList is the player page's right-hand "今日推薦" column: a vertical
// list of recommended videos (same source as the home page). Mirrors UpNextList's
// row style; RecommendationItem has no tags, so the tag line is omitted.
export default function RecommendationList({ items }: { items: RecommendationItem[] }) {
  if (items.length === 0) return null
  return (
    <div className="space-y-1">
      {items.map((rec) => (
        <Link
          key={rec.id}
          to={`/videos/${rec.video_id}`}
          className="group flex gap-3 rounded-card p-2 transition-colors hover:bg-surface"
        >
          <PosterThumb
            id={rec.video_id}
            title={rec.title}
            thumbnailUrl={rec.thumbnail_url}
            showFallbackTitle={false}
            className="aspect-video w-[140px] shrink-0 rounded-lg"
          >
            <span className="absolute bottom-1 right-1 rounded bg-black/70 px-1 py-0.5 font-mono text-[10px] text-cream">
              {formatDuration(rec.duration_seconds)}
            </span>
          </PosterThumb>
          <div className="min-w-0 flex-1 py-0.5">
            <h4 className="line-clamp-2 text-sm font-medium leading-snug text-cream transition-colors group-hover:text-accent">
              {rec.title}
            </h4>
            <div className="mt-1 font-mono text-[11px] text-muted">
              <span className="text-accent">{rec.resolution}</span>
            </div>
          </div>
        </Link>
      ))}
    </div>
  )
}
