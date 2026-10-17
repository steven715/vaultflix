import { useState } from 'react'
import { Link } from 'react-router-dom'
import { listFavorites, removeFavorite } from '../api/favorites'
import type { VideoSummaryWithURL } from '../types'
import AppShell, { Container } from '../components/AppShell'
import PosterThumb from '../components/PosterThumb'
import Pagination from '../components/Pagination'
import Skeleton from '../components/Skeleton'
import ErrorBanner from '../components/ErrorBanner'
import EmptyState from '../components/EmptyState'
import { useToast } from '../contexts/ToastContext'
import { usePagedList } from '../hooks/usePagedList'
import { formatDuration, formatFileSize } from '../utils/format'
import { HeartIcon, HeartFilled, PlayIcon } from '../components/icons'

const PAGE_SIZE = 20

export default function FavoritesPage() {
  const [page, setPage] = useState(1)
  const toast = useToast()
  const list = usePagedList<VideoSummaryWithURL>({ load: (p) => listFavorites(p, PAGE_SIZE), page, pageSize: PAGE_SIZE, setPage })
  const { items: favorites, total, totalPages } = list

  async function handleRemove(videoID: string) {
    // Optimistic UI: remove immediately
    list.mutate((p) => ({ items: p.items.filter((f) => f.id !== videoID), total: p.total - 1 }))
    try {
      await removeFavorite(videoID)
    } catch {
      toast.error('取消收藏失敗')
      list.reload().catch(() => toast.error('重新載入收藏失敗'))
    }
  }

  return (
    <AppShell>
      <Container className="py-8">
        <h1 className="mb-6 font-display text-3xl font-extrabold tracking-tight text-cream">
          我的收藏
          {total > 0 && <span className="ml-2 font-mono text-base text-faint">{total}</span>}
        </h1>

        {list.status === 'loading' ? (
          <div className="grid grid-cols-2 gap-x-4 gap-y-6 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5">
            {Array.from({ length: 10 }).map((_, i) => (
              <div key={i}>
                <Skeleton className="aspect-[16/10]" />
                <Skeleton className="mt-2.5 h-4 w-3/4 rounded" />
              </div>
            ))}
          </div>
        ) : list.status === 'error' ? (
          <ErrorBanner
            message="無法載入收藏，請確認服務是否正常運作"
            onRetry={list.retry}
          />
        ) : favorites.length === 0 ? (
          <EmptyState
            icon={<HeartIcon className="h-full w-full" />}
            title="還沒有收藏"
            description="瀏覽片庫，把喜歡的影片加入收藏，之後就能在這裡快速找到。"
            ctaLabel="去逛片庫"
            ctaTo="/"
          />
        ) : (
          <>
            <div className="grid grid-cols-2 gap-x-4 gap-y-6 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5">
              {favorites.map((video) => (
                <div key={video.id} className="group block transition-transform duration-200 hover:-translate-y-[5px]">
                  <Link to={`/videos/${video.id}`} className="block">
                    <PosterThumb
                      id={video.id}
                      title={video.title}
                      thumbnailUrl={video.thumbnail_url}
                      className="aspect-[16/10] rounded-card"
                    >
                      <div className="absolute inset-0 flex items-center justify-center opacity-0 transition-opacity duration-200 group-hover:opacity-100">
                        <span className="flex h-12 w-12 items-center justify-center rounded-full border border-white/20 bg-black/40 backdrop-blur-sm">
                          <PlayIcon className="ml-0.5 h-5 w-5 text-cream" />
                        </span>
                      </div>
                      <span className="absolute bottom-2 right-2 rounded-md bg-black/70 px-1.5 py-0.5 font-mono text-[11px] text-cream">
                        {formatDuration(video.duration_seconds)}
                      </span>
                      <button
                        onClick={(e) => {
                          e.preventDefault()
                          e.stopPropagation()
                          handleRemove(video.id)
                        }}
                        title="取消收藏"
                        className="absolute right-2 top-2 flex h-8 w-8 items-center justify-center rounded-full border border-white/15 bg-black/45 text-fav backdrop-blur-sm transition-transform hover:scale-110"
                      >
                        <HeartFilled className="h-4 w-4" />
                      </button>
                    </PosterThumb>
                    <h3 className="mt-2.5 line-clamp-2 text-sm font-semibold leading-snug text-cream transition-colors group-hover:text-accent">
                      {video.title}
                    </h3>
                    <div className="mt-1 flex items-center gap-1.5 font-mono text-[11px] text-muted">
                      <span className="text-accent">{video.resolution}</span>
                      <span className="text-faint">·</span>
                      <span>{formatFileSize(video.file_size_bytes)}</span>
                    </div>
                  </Link>
                </div>
              ))}
            </div>

            <Pagination page={page} totalPages={totalPages} onPageChange={setPage} />
          </>
        )}
      </Container>
    </AppShell>
  )
}
