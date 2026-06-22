import { useState, useEffect, useCallback } from 'react'
import { useSearchParams, useLocation } from 'react-router-dom'
import { listVideos } from '../api/videos'
import { getTodayRecommendations } from '../api/recommendations'
import { listWatchHistory } from '../api/watchHistory'
import type { VideoWithTags, RecommendationItem, WatchHistoryItem } from '../types'
import AppShell, { Container } from '../components/AppShell'
import BrowseHero from '../components/BrowseHero'
import MediaRail from '../components/MediaRail'
import ContinueCard from '../components/ContinueCard'
import RecommendationCard from '../components/RecommendationCard'
import MobileTopBar from '../components/MobileTopBar'
import MobileRankList from '../components/MobileRankList'
import TagFilterBar from '../components/TagFilterBar'
import VideoCard from '../components/VideoCard'
import Pagination from '../components/Pagination'
import Skeleton from '../components/Skeleton'
import ErrorBanner from '../components/ErrorBanner'

const SORT_OPTIONS = [
  { value: 'created_at', label: '最新' },
  { value: 'title', label: '標題' },
  { value: 'duration_seconds', label: '時長' },
  { value: 'file_size_bytes', label: '大小' },
]

export default function BrowsePage() {
  const [searchParams, setSearchParams] = useSearchParams()
  const location = useLocation()
  const [videos, setVideos] = useState<VideoWithTags[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState(false)
  const [reloadKey, setReloadKey] = useState(0)
  const [recommendations, setRecommendations] = useState<RecommendationItem[]>([])
  const [continueWatching, setContinueWatching] = useState<WatchHistoryItem[]>([])

  const page = Number(searchParams.get('page')) || 1
  const pageSize = Number(searchParams.get('page_size')) || 20
  const sortBy = searchParams.get('sort_by') || 'created_at'
  const sortOrder = searchParams.get('sort_order') || 'desc'
  const query = searchParams.get('q') || ''
  const tagIdsStr = searchParams.get('tag_ids') || ''
  const selectedTagIds = tagIdsStr ? tagIdsStr.split(',').map(Number) : []

  const totalPages = Math.ceil(total / pageSize)
  const isHome = !query && selectedTagIds.length === 0
  const showHero = isHome && page === 1 && videos.length > 0
  const heroVideo = showHero ? videos[0] : null
  const gridVideos = showHero ? videos.slice(1) : videos

  const updateParams = useCallback(
    (updates: Record<string, string>) => {
      setSearchParams((prev) => {
        const next = new URLSearchParams(prev)
        for (const [key, value] of Object.entries(updates)) {
          if (value) next.set(key, value)
          else next.delete(key)
        }
        return next
      })
    },
    [setSearchParams],
  )

  // Browse rails (recommendations + continue watching) only on the home view.
  // location.key changes on every navigation to "/" — including clicking the
  // Vaultflix logo / 首頁 while already on home — so the rails refresh each time.
  useEffect(() => {
    if (query) return
    let cancelled = false
    getTodayRecommendations()
      .then((items) => !cancelled && setRecommendations(items))
      .catch((err) => console.warn('failed to load recommendations', err))
    listWatchHistory(1, 12)
      .then((res) => {
        if (cancelled) return
        setContinueWatching(res.data.filter((h) => !h.completed && h.progress_seconds > 0))
      })
      .catch((err) => console.warn('failed to load watch history', err))
    return () => {
      cancelled = true
    }
  }, [query, reloadKey, location.key])

  useEffect(() => {
    let cancelled = false
    const load = async () => {
      setLoading(true)
      setLoadError(false)
      try {
        const res = await listVideos({
          page,
          page_size: pageSize,
          sort_by: sortBy,
          sort_order: sortOrder,
          q: query || undefined,
          tag_ids: tagIdsStr || undefined,
        })
        if (cancelled) return
        setVideos(res.data)
        setTotal(res.total)
      } catch {
        if (cancelled) return
        setVideos([])
        setTotal(0)
        setLoadError(true)
      } finally {
        if (!cancelled) setLoading(false)
      }
    }
    load()
    return () => {
      cancelled = true
    }
  }, [page, pageSize, sortBy, sortOrder, query, tagIdsStr, reloadKey])

  return (
    <AppShell>
      {/* Desktop full-bleed hero; mobile uses the magazine layout below */}
      {heroVideo && (
        <div className="hidden md:block">
          <BrowseHero video={heroVideo} />
        </div>
      )}

      <Container className="py-8">
        {isHome && page === 1 && <MobileTopBar />}

        {!query && continueWatching.length > 0 && (
          <div className="mt-6 md:mt-0">
            <MediaRail title="繼續觀看">
              {continueWatching.map((item) => (
                <ContinueCard key={item.id} item={item} />
              ))}
            </MediaRail>
          </div>
        )}

        {!query && recommendations.length > 0 && (
          <>
            {/* Desktop: horizontal rail */}
            <div className="hidden md:block">
              <MediaRail title={recommendations[0].is_fallback ? '為你推薦' : '今日推薦'}>
                {recommendations.map((rec) => (
                  <RecommendationCard key={rec.id} rec={rec} />
                ))}
              </MediaRail>
            </div>
            {/* Mobile (variant B): numbered editorial ranking */}
            <section className="mb-9 md:hidden">
              <h2 className="mb-4 font-display text-[34px] font-extrabold leading-none tracking-tight text-cream">
                {recommendations[0].is_fallback ? '為你推薦' : '今日推薦'}
              </h2>
              <MobileRankList items={recommendations} />
            </section>
          </>
        )}

        {/* Library / search results */}
        <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
          <h2 className="font-display text-2xl font-bold tracking-tight text-cream">
            {query ? `「${query}」的結果` : '片庫'}
            {total > 0 && <span className="ml-2 font-mono text-sm text-faint">{total}</span>}
          </h2>
          {!query && (
            <div className="no-scrollbar flex items-center gap-2 overflow-x-auto">
              {SORT_OPTIONS.map((opt) => {
                const active = sortBy === opt.value
                return (
                  <button
                    key={opt.value}
                    onClick={() => updateParams({ sort_by: opt.value, page: '1' })}
                    className={`shrink-0 rounded-pill border px-3 py-1.5 text-sm font-medium transition-colors ${
                      active
                        ? 'border-accent/50 bg-accent/15 text-accent'
                        : 'border-border bg-surface text-muted hover:text-cream'
                    }`}
                  >
                    {opt.label}
                  </button>
                )
              })}
              <button
                onClick={() => updateParams({ sort_order: sortOrder === 'asc' ? 'desc' : 'asc', page: '1' })}
                className="flex h-8 w-8 shrink-0 items-center justify-center rounded-pill border border-border bg-surface text-muted transition-colors hover:text-cream"
                title={sortOrder === 'asc' ? '升序' : '降序'}
              >
                {sortOrder === 'asc' ? '↑' : '↓'}
              </button>
            </div>
          )}
        </div>

        {!query && (
          <div className="mb-6">
            <TagFilterBar
              selectedTagIds={selectedTagIds}
              onTagsChange={(ids) => updateParams({ tag_ids: ids.join(','), page: '1' })}
            />
          </div>
        )}

        {loading ? (
          <div className="grid grid-cols-2 gap-x-4 gap-y-6 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5">
            {Array.from({ length: 10 }).map((_, i) => (
              <div key={i}>
                <Skeleton className="aspect-[16/10]" />
                <Skeleton className="mt-2.5 h-4 w-3/4 rounded" />
                <Skeleton className="mt-2 h-3 w-1/2 rounded" />
              </div>
            ))}
          </div>
        ) : loadError ? (
          <ErrorBanner
            message="無法載入影片，請確認服務是否正常運作"
            onRetry={() => setReloadKey((k) => k + 1)}
          />
        ) : gridVideos.length === 0 ? (
          <div className="py-20 text-center text-muted">
            {query || tagIdsStr ? '沒有符合條件的影片' : '尚無影片'}
          </div>
        ) : (
          <div className="grid grid-cols-2 gap-x-4 gap-y-6 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5">
            {gridVideos.map((video) => (
              <VideoCard key={video.id} video={video} />
            ))}
          </div>
        )}

        <Pagination page={page} totalPages={totalPages} onPageChange={(p) => updateParams({ page: String(p) })} />
      </Container>
    </AppShell>
  )
}
