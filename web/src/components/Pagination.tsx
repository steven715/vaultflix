interface PaginationProps {
  page: number
  totalPages: number
  onPageChange: (page: number) => void
}

export default function Pagination({ page, totalPages, onPageChange }: PaginationProps) {
  if (totalPages <= 1) return null

  function getPageNumbers(): (number | '...')[] {
    const pages: (number | '...')[] = []
    if (totalPages <= 7) {
      for (let i = 1; i <= totalPages; i++) pages.push(i)
      return pages
    }

    pages.push(1)
    if (page > 3) pages.push('...')

    const start = Math.max(2, page - 1)
    const end = Math.min(totalPages - 1, page + 1)
    for (let i = start; i <= end; i++) pages.push(i)

    if (page < totalPages - 2) pages.push('...')
    pages.push(totalPages)

    return pages
  }

  return (
    <div className="flex items-center justify-center gap-1.5 py-8">
      <button
        onClick={() => onPageChange(page - 1)}
        disabled={page <= 1}
        className="rounded-btn border border-border bg-surface px-3.5 py-1.5 text-sm text-muted transition-colors hover:text-cream disabled:cursor-not-allowed disabled:opacity-30"
      >
        上一頁
      </button>
      {getPageNumbers().map((p, i) =>
        p === '...' ? (
          <span key={`ellipsis-${i}`} className="px-2 text-faint">…</span>
        ) : (
          <button
            key={p}
            onClick={() => onPageChange(p)}
            className={`min-w-9 rounded-btn px-3 py-1.5 font-mono text-sm transition-colors ${
              p === page
                ? 'bg-accent text-accent-ink'
                : 'border border-border bg-surface text-muted hover:text-cream'
            }`}
          >
            {p}
          </button>
        ),
      )}
      <button
        onClick={() => onPageChange(page + 1)}
        disabled={page >= totalPages}
        className="rounded-btn border border-border bg-surface px-3.5 py-1.5 text-sm text-muted transition-colors hover:text-cream disabled:cursor-not-allowed disabled:opacity-30"
      >
        下一頁
      </button>
    </div>
  )
}
