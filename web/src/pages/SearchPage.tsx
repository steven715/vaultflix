import { useState, useEffect, useRef } from 'react'
import { useSearchParams, useNavigate } from 'react-router-dom'
import { listVideos } from '../api/videos'
import { listTags } from '../api/tags'
import type { VideoWithTags, TagWithCount } from '../types'
import AppShell, { Container } from '../components/AppShell'
import VideoCard from '../components/VideoCard'
import Skeleton from '../components/Skeleton'
import EmptyState from '../components/EmptyState'
import { SearchIcon } from '../components/icons'

const RECENT_KEY = 'vaultflix-recent-searches'

function loadRecent(): string[] {
  try {
    return JSON.parse(localStorage.getItem(RECENT_KEY) || '[]')
  } catch {
    return []
  }
}

export default function SearchPage() {
  const [searchParams, setSearchParams] = useSearchParams()
  const navigate = useNavigate()
  const query = searchParams.get('q') || ''
  const [value, setValue] = useState(query)
  const [results, setResults] = useState<VideoWithTags[]>([])
  const [loading, setLoading] = useState(false)
  const [tags, setTags] = useState<TagWithCount[]>([])
  const [recent, setRecent] = useState<string[]>(loadRecent)
  const debounceRef = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)

  useEffect(() => {
    setValue(query)
  }, [query])

  // Popular tags for the empty state.
  useEffect(() => {
    let cancelled = false
    listTags()
      .then((t) => {
        if (!cancelled) setTags(t.filter((x) => x.video_count > 0).sort((a, b) => b.video_count - a.video_count).slice(0, 12))
      })
      .catch((err) => console.warn('failed to load tags', err))
    return () => {
      cancelled = true
    }
  }, [])

  // Run the search whenever the URL query changes.
  useEffect(() => {
    if (!query) {
      setResults([])
      return
    }
    let cancelled = false
    setLoading(true)
    listVideos({ page: 1, page_size: 40, q: query })
      .then((res) => !cancelled && setResults(res.data))
      .catch(() => !cancelled && setResults([]))
      .finally(() => !cancelled && setLoading(false))
    return () => {
      cancelled = true
    }
  }, [query])

  function commit(next: string) {
    const params = new URLSearchParams(searchParams)
    if (next) params.set('q', next)
    else params.delete('q')
    setSearchParams(params, { replace: true })
  }

  function handleChange(next: string) {
    setValue(next)
    clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(() => commit(next), 300)
  }

  function rememberAndGo(term: string) {
    const next = [term, ...recent.filter((r) => r !== term)].slice(0, 8)
    setRecent(next)
    localStorage.setItem(RECENT_KEY, JSON.stringify(next))
    commit(term)
  }

  return (
    <AppShell>
      <Container className="py-6">
        {/* Search field */}
        <div className="relative">
          <SearchIcon className="pointer-events-none absolute left-4 top-1/2 h-5 w-5 -translate-y-1/2 text-faint" />
          <input
            autoFocus
            type="text"
            value={value}
            onChange={(e) => handleChange(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && value.trim()) rememberAndGo(value.trim())
            }}
            placeholder="搜尋影片、標籤…"
            className="h-12 w-full rounded-pill border border-border bg-surface pl-12 pr-4 text-cream outline-none transition-colors placeholder:text-faint focus:border-accent"
          />
        </div>

        {!query ? (
          <div className="mt-8 space-y-8">
            {recent.length > 0 && (
              <section>
                <div className="mb-3 flex items-center justify-between">
                  <h2 className="text-sm font-medium uppercase tracking-wider text-muted">最近搜尋</h2>
                  <button
                    onClick={() => {
                      setRecent([])
                      localStorage.removeItem(RECENT_KEY)
                    }}
                    className="text-xs text-faint hover:text-muted"
                  >
                    清除
                  </button>
                </div>
                <div className="flex flex-col">
                  {recent.map((term) => (
                    <button
                      key={term}
                      onClick={() => rememberAndGo(term)}
                      className="flex items-center gap-3 rounded-card px-2 py-2.5 text-left text-sm text-cream transition-colors hover:bg-surface"
                    >
                      <SearchIcon className="h-4 w-4 text-faint" />
                      {term}
                    </button>
                  ))}
                </div>
              </section>
            )}

            {tags.length > 0 && (
              <section>
                <h2 className="mb-3 text-sm font-medium uppercase tracking-wider text-muted">熱門標籤</h2>
                <div className="flex flex-wrap gap-2">
                  {tags.map((tag) => (
                    <button
                      key={tag.id}
                      onClick={() => navigate(`/?tag_ids=${tag.id}`)}
                      className="rounded-pill border border-border bg-surface px-3.5 py-1.5 text-sm text-muted transition-colors hover:bg-surface-up hover:text-cream"
                    >
                      {tag.name}
                      <span className="text-faint"> {tag.video_count}</span>
                    </button>
                  ))}
                </div>
              </section>
            )}
          </div>
        ) : loading ? (
          <div className="mt-6 grid grid-cols-2 gap-x-4 gap-y-6 sm:grid-cols-3 lg:grid-cols-4">
            {Array.from({ length: 8 }).map((_, i) => (
              <div key={i}>
                <Skeleton className="aspect-[16/10]" />
                <Skeleton className="mt-2.5 h-4 w-3/4 rounded" />
              </div>
            ))}
          </div>
        ) : results.length === 0 ? (
          <EmptyState
            icon={<SearchIcon className="h-full w-full" />}
            title="找不到結果"
            description={`沒有符合「${query}」的影片，換個關鍵字或標籤試試。`}
          />
        ) : (
          <div className="mt-6 grid grid-cols-2 gap-x-4 gap-y-6 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5">
            {results.map((video) => (
              <VideoCard key={video.id} video={video} />
            ))}
          </div>
        )}
      </Container>
    </AppShell>
  )
}
