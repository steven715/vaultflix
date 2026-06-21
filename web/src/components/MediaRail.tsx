import { useRef, type ReactNode } from 'react'
import { ChevronLeft, ChevronRight } from './icons'

interface MediaRailProps {
  title: string
  // Optional count rendered in mono next to the title (e.g. library total).
  count?: number
  children: ReactNode
}

// MediaRail is a titled, horizontally scrollable row with arrow buttons that
// scroll by a viewport-ish amount. Used by "繼續觀看" and "為你推薦".
export default function MediaRail({ title, count, children }: MediaRailProps) {
  const scrollRef = useRef<HTMLDivElement>(null)

  function scrollBy(delta: number) {
    scrollRef.current?.scrollBy({ left: delta, behavior: 'smooth' })
  }

  return (
    <section className="mb-10">
      <div className="mb-3.5 flex items-center justify-between">
        <h2 className="font-display text-2xl font-bold tracking-tight text-cream">
          {title}
          {count !== undefined && <span className="ml-2 font-mono text-sm text-faint">{count}</span>}
        </h2>
        <div className="hidden items-center gap-2 md:flex">
          <button
            onClick={() => scrollBy(-640)}
            className="flex h-9 w-9 items-center justify-center rounded-full border border-border bg-surface text-muted transition-colors hover:bg-surface-up hover:text-cream"
            aria-label="向左捲動"
          >
            <ChevronLeft className="h-4 w-4" />
          </button>
          <button
            onClick={() => scrollBy(640)}
            className="flex h-9 w-9 items-center justify-center rounded-full border border-border bg-surface text-muted transition-colors hover:bg-surface-up hover:text-cream"
            aria-label="向右捲動"
          >
            <ChevronRight className="h-4 w-4" />
          </button>
        </div>
      </div>
      <div ref={scrollRef} className="no-scrollbar flex gap-4 overflow-x-auto pb-2">
        {children}
      </div>
    </section>
  )
}
