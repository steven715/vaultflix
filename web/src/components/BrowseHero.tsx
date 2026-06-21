import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import type { VideoWithTags } from '../types'
import { addFavorite } from '../api/favorites'
import { useToast } from '../contexts/ToastContext'
import { formatDuration } from '../utils/format'
import { posterGradient } from '../utils/poster'
import { PlayIcon, HeartIcon } from './icons'

// BrowseHero is the full-bleed featured banner at the top of the browse page.
export default function BrowseHero({ video }: { video: VideoWithTags }) {
  const navigate = useNavigate()
  const toast = useToast()
  const [saving, setSaving] = useState(false)

  async function addToList() {
    if (saving) return
    setSaving(true)
    try {
      await addFavorite(video.id)
      toast.success('已加入收藏')
    } catch {
      toast.error('加入收藏失敗')
    } finally {
      setSaving(false)
    }
  }

  const bg = video.thumbnail_url
    ? `url(${video.thumbnail_url})`
    : posterGradient(video.id)

  return (
    <section className="relative h-[420px] w-full overflow-hidden md:h-[520px]">
      <div
        className="absolute inset-0 bg-cover bg-center"
        style={{ backgroundImage: bg }}
      />
      {/* Left darkening + bottom fade */}
      <div className="absolute inset-0 bg-gradient-to-r from-bg via-bg/70 to-transparent" />
      <div className="absolute inset-0 bg-gradient-to-t from-bg via-transparent to-transparent" />

      <div className="absolute inset-0 flex items-end">
        <div className="mx-auto w-full max-w-[1360px] px-4 pb-12 sm:px-6 lg:px-10">
          <div className="max-w-[560px] animate-fade-up">
            <span className="inline-block rounded-pill bg-accent px-3 py-1 text-xs font-bold text-accent-ink">
              今日精選
            </span>
            <h1 className="mt-4 font-display text-4xl font-extrabold leading-[1.05] tracking-tight text-cream md:text-[56px]">
              {video.title}
            </h1>
            <div className="mt-4 flex flex-wrap items-center gap-x-3 gap-y-1 font-mono text-xs text-muted">
              <span className="text-accent">{video.resolution}</span>
              <span className="text-faint">·</span>
              <span>{formatDuration(video.duration_seconds)}</span>
              {video.tags.slice(0, 2).map((t) => (
                <span key={t.id} className="text-faint">
                  · {t.name}
                </span>
              ))}
            </div>
            {video.description && (
              <p className="mt-4 line-clamp-3 max-w-[520px] text-sm leading-relaxed text-muted">
                {video.description}
              </p>
            )}
            <div className="mt-6 flex items-center gap-3">
              <button
                onClick={() => navigate(`/videos/${video.id}`)}
                className="flex items-center gap-2 rounded-btn bg-cream px-6 py-3 text-sm font-bold text-bg transition-transform hover:-translate-y-0.5"
              >
                <PlayIcon className="h-5 w-5" />
                立即播放
              </button>
              <button
                onClick={addToList}
                disabled={saving}
                className="rounded-btn border border-border bg-white/5 px-5 py-3 text-sm font-medium text-cream backdrop-blur-sm transition-colors hover:bg-white/10 disabled:opacity-50"
              >
                加入清單
              </button>
              <button
                onClick={addToList}
                disabled={saving}
                aria-label="收藏"
                className="flex h-12 w-12 items-center justify-center rounded-full border border-border bg-white/5 text-cream backdrop-blur-sm transition-colors hover:bg-white/10 disabled:opacity-50"
              >
                <HeartIcon className="h-5 w-5" />
              </button>
            </div>
          </div>
        </div>
      </div>
    </section>
  )
}
