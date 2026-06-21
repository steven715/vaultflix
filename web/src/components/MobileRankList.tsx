import { Link } from 'react-router-dom'
import type { RecommendationItem } from '../types'
import { formatDuration } from '../utils/format'
import PosterThumb from './PosterThumb'
import { ChevronRight } from './icons'

// MobileRankList is the magazine-style numbered ranking (variant B) shown on the
// mobile home: outlined index number + small thumb + title/meta + chevron. Rows
// reveal with a staggered "row-in" entrance and fill accent on press.
export default function MobileRankList({ items }: { items: RecommendationItem[] }) {
  return (
    <ol className="space-y-1">
      {items.slice(0, 7).map((rec, i) => (
        <li key={rec.id} className="animate-row-in" style={{ animationDelay: `${i * 80}ms` }}>
          <Link
            to={`/videos/${rec.video_id}`}
            className="group flex items-center gap-3 rounded-card p-2 transition-[transform,background-color] duration-150 active:scale-[0.975] active:bg-accent/10"
          >
            <span className="w-9 shrink-0 text-center font-display text-3xl font-extrabold text-transparent transition-colors duration-150 [-webkit-text-stroke:1px_var(--color-faint)] group-active:text-accent group-active:[-webkit-text-stroke:1px_var(--color-accent)]">
              {String(i + 1).padStart(2, '0')}
            </span>
            <PosterThumb
              id={rec.video_id}
              title={rec.title}
              thumbnailUrl={rec.thumbnail_url}
              showFallbackTitle={false}
              className="aspect-[92/58] w-[92px] shrink-0 rounded-lg"
            />
            <div className="min-w-0 flex-1">
              <h3 className="line-clamp-1 text-sm font-semibold text-cream">{rec.title}</h3>
              <div className="mt-0.5 font-mono text-[11px] text-muted">
                <span className="text-accent">{rec.resolution}</span>
                <span className="text-faint"> · {formatDuration(rec.duration_seconds)}</span>
              </div>
            </div>
            <ChevronRight className="h-4 w-4 shrink-0 text-faint" />
          </Link>
        </li>
      ))}
    </ol>
  )
}
