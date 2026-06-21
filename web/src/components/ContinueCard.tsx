import { Link } from 'react-router-dom'
import type { WatchHistoryItem } from '../types'
import { formatDuration } from '../utils/format'
import PosterThumb from './PosterThumb'
import { PlayIcon } from './icons'

// ContinueCard is a wide "繼續觀看" rail card: poster with a duration badge, a
// bottom accent progress bar, and a "剩餘 mm:ss" label below.
export default function ContinueCard({ item }: { item: WatchHistoryItem }) {
  const remaining = Math.max(0, item.duration_seconds - item.progress_seconds)
  const pct =
    item.duration_seconds > 0
      ? Math.min(100, (item.progress_seconds / item.duration_seconds) * 100)
      : 0

  return (
    <Link
      to={`/videos/${item.video_id}`}
      className="group block w-[280px] shrink-0 transition-transform duration-200 hover:-translate-y-[5px] sm:w-[340px]"
    >
      <PosterThumb
        id={item.video_id}
        title={item.title}
        thumbnailUrl={item.thumbnail_url}
        className="aspect-video rounded-card"
      >
        <div className="absolute inset-0 flex items-center justify-center opacity-0 transition-opacity duration-200 group-hover:opacity-100">
          <span className="flex h-12 w-12 items-center justify-center rounded-full border border-white/20 bg-black/40 backdrop-blur-sm">
            <PlayIcon className="ml-0.5 h-5 w-5 text-cream" />
          </span>
        </div>
        <span className="absolute right-2 top-2 rounded-md bg-black/70 px-1.5 py-0.5 font-mono text-[11px] text-cream">
          {formatDuration(item.duration_seconds)}
        </span>
        {/* Bottom progress bar */}
        <div className="absolute inset-x-0 bottom-0 h-[5px] bg-black/40">
          <div className="h-full bg-accent" style={{ width: `${pct}%` }} />
        </div>
      </PosterThumb>
      <h3 className="mt-2.5 line-clamp-1 text-sm font-semibold text-cream transition-colors group-hover:text-accent">
        {item.title}
      </h3>
      <p className="mt-1 font-mono text-[11px] text-muted">
        {item.completed ? '已看完' : `剩餘 ${formatDuration(remaining)}`}
      </p>
    </Link>
  )
}
