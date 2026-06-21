import { useState, useEffect } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { listWatchHistory } from '../api/watchHistory'
import type { WatchHistoryItem } from '../types'
import AppShell, { Container } from '../components/AppShell'
import PosterThumb from '../components/PosterThumb'
import Pagination from '../components/Pagination'
import Skeleton from '../components/Skeleton'
import ErrorBanner from '../components/ErrorBanner'
import EmptyState from '../components/EmptyState'
import { formatDuration, formatRelativeTime, historyGroup, type HistoryGroup } from '../utils/format'
import { ClockIcon, PlayIcon, CloseIcon } from '../components/icons'

const PAGE_SIZE = 20
const GROUP_ORDER: HistoryGroup[] = ['今天', '本週', '更早']

export default function HistoryPage() {
  const navigate = useNavigate()
  const [items, setItems] = useState<WatchHistoryItem[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState(false)
  const [reloadKey, setReloadKey] = useState(0)

  const totalPages = Math.ceil(total / PAGE_SIZE)

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setLoadError(false)

    listWatchHistory(page, PAGE_SIZE)
      .then((res) => {
        if (cancelled) return
        setItems(res.data)
        setTotal(res.total)
      })
      .catch(() => {
        if (cancelled) return
        setItems([])
        setTotal(0)
        setLoadError(true)
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })

    return () => {
      cancelled = true
    }
  }, [page, reloadKey])

  // Remove a single row from the visible list (local-only, per the design).
  function handleRemove(id: string) {
    setItems((prev) => prev.filter((i) => i.id !== id))
  }

  const groups = GROUP_ORDER.map((g) => ({
    group: g,
    rows: items.filter((i) => historyGroup(i.watched_at) === g),
  })).filter((g) => g.rows.length > 0)

  return (
    <AppShell>
      <Container width="narrow" className="py-8">
        <h1 className="mb-6 font-display text-3xl font-extrabold tracking-tight text-cream">觀看記錄</h1>

        {loading ? (
          <div className="space-y-3">
            {Array.from({ length: 6 }).map((_, i) => (
              <div key={i} className="flex gap-4">
                <Skeleton className="aspect-video w-[168px] shrink-0" />
                <div className="flex-1 space-y-2 py-2">
                  <Skeleton className="h-4 w-2/3 rounded" />
                  <Skeleton className="h-3 w-1/3 rounded" />
                </div>
              </div>
            ))}
          </div>
        ) : loadError ? (
          <ErrorBanner
            message="無法載入觀看記錄，請確認服務是否正常運作"
            onRetry={() => setReloadKey((k) => k + 1)}
          />
        ) : items.length === 0 ? (
          <EmptyState
            icon={<ClockIcon className="h-full w-full" />}
            title="還沒有觀看記錄"
            description="開始觀看影片後，這裡會記錄你的進度，方便隨時繼續。"
            ctaLabel="去逛片庫"
            ctaTo="/"
          />
        ) : (
          <>
            {groups.map(({ group, rows }) => (
              <section key={group} className="mb-8">
                <h2 className="mb-3 text-xs font-medium uppercase tracking-[1.5px] text-muted">{group}</h2>
                <div className="space-y-1">
                  {rows.map((item) => {
                    const pct =
                      item.duration_seconds > 0
                        ? Math.min(100, (item.progress_seconds / item.duration_seconds) * 100)
                        : 0
                    return (
                      <div
                        key={item.id}
                        className="group flex items-center gap-4 rounded-card p-2 transition-colors hover:bg-surface"
                      >
                        <Link to={`/videos/${item.video_id}`} className="shrink-0">
                          <PosterThumb
                            id={item.video_id}
                            title={item.title}
                            thumbnailUrl={item.thumbnail_url}
                            showFallbackTitle={false}
                            className="aspect-video w-[140px] rounded-lg sm:w-[168px]"
                          >
                            <div className="absolute inset-0 flex items-center justify-center opacity-0 transition-opacity group-hover:opacity-100">
                              <span className="flex h-10 w-10 items-center justify-center rounded-full border border-white/20 bg-black/40 backdrop-blur-sm">
                                <PlayIcon className="ml-0.5 h-4 w-4 text-cream" />
                              </span>
                            </div>
                            <div className="absolute inset-x-0 bottom-0 h-1 bg-black/40">
                              <div className="h-full bg-accent" style={{ width: `${pct}%` }} />
                            </div>
                          </PosterThumb>
                        </Link>

                        <div className="min-w-0 flex-1">
                          <Link to={`/videos/${item.video_id}`}>
                            <h3 className="truncate text-sm font-semibold text-cream transition-colors group-hover:text-accent">
                              {item.title}
                            </h3>
                          </Link>
                          <p className="mt-1 font-mono text-[11px] text-muted">
                            {formatRelativeTime(item.watched_at)} ·{' '}
                            {item.completed
                              ? '已看完'
                              : `已看 ${formatDuration(item.progress_seconds)} / ${formatDuration(item.duration_seconds)}`}
                          </p>
                        </div>

                        <button
                          onClick={() => navigate(`/videos/${item.video_id}`)}
                          className="hidden shrink-0 rounded-pill border border-border bg-surface px-3.5 py-1.5 text-sm font-medium text-muted transition-colors hover:border-accent hover:text-accent sm:block"
                        >
                          繼續
                        </button>
                        <button
                          onClick={() => handleRemove(item.id)}
                          aria-label="移除"
                          className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full text-faint transition-colors hover:bg-surface-up hover:text-cream"
                        >
                          <CloseIcon className="h-4 w-4" />
                        </button>
                      </div>
                    )
                  })}
                </div>
              </section>
            ))}

            <Pagination page={page} totalPages={totalPages} onPageChange={setPage} />
          </>
        )}
      </Container>
    </AppShell>
  )
}
