import type { LibrarySortBy, SortOrder } from '../../lib/libraryParams'
import type { TagWithCount } from '../../types'
import { parseTagIds, serializeTagIds, toggleTagId, toggleSort } from '../../lib/libraryParams'

interface SortChip {
  label: string
  value: LibrarySortBy
}

const SORT_CHIPS: SortChip[] = [
  { label: '最新', value: 'created_at' },
  { label: '標題', value: 'title' },
  { label: '時長', value: 'duration_seconds' },
  { label: '大小', value: 'file_size_bytes' },
]

interface LibraryToolbarProps {
  total: number
  sortBy: LibrarySortBy
  sortOrder: SortOrder
  tagIdsStr: string
  allTags: TagWithCount[]
  selected: string[]
  backfillStarting: boolean
  backfillJobId: string | null
  onSort: (params: { sort_by: string; sort_order: string }) => void
  onTagFilter: (tag_ids: string) => void
  onStartBackfill: () => void
  onOpenImport: () => void
  onBatchTag: () => void
  onBatchDelete: () => void
  onClearSelection: () => void
}

export default function LibraryToolbar({
  total,
  sortBy,
  sortOrder,
  tagIdsStr,
  allTags,
  selected,
  backfillStarting,
  backfillJobId,
  onSort,
  onTagFilter,
  onStartBackfill,
  onOpenImport,
  onBatchTag,
  onBatchDelete,
  onClearSelection,
}: LibraryToolbarProps) {
  const selectedTagIds = parseTagIds(tagIdsStr)

  function handleSortChip(value: LibrarySortBy) {
    const next = toggleSort({ by: sortBy, order: sortOrder }, value)
    onSort({ sort_by: next.by, sort_order: next.order })
  }

  function handleTagChip(id: number | null) {
    if (id === null) {
      onTagFilter('')
      return
    }
    const next = toggleTagId(selectedTagIds, id)
    onTagFilter(serializeTagIds(next))
  }

  return (
    <div className="mb-4 space-y-2">
      {/* Row 1: count + sort + actions */}
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-muted text-sm shrink-0">共 {total} 部</span>
        <div className="flex items-center gap-1 flex-wrap">
          {SORT_CHIPS.map((chip) => {
            const active = sortBy === chip.value
            return (
              <button
                key={chip.value}
                onClick={() => handleSortChip(chip.value)}
                className={`text-xs px-2.5 py-1 rounded-btn transition-colors ${
                  active
                    ? 'bg-accent text-accent-ink'
                    : 'bg-surface-2 text-muted hover:text-cream'
                }`}
              >
                {chip.label}
                {active && (
                  <span className="ml-1">{sortOrder === 'asc' ? '↑' : '↓'}</span>
                )}
              </button>
            )
          })}
        </div>
        <div className="ml-auto flex gap-2 shrink-0">
          <button
            onClick={onStartBackfill}
            disabled={backfillStarting || backfillJobId !== null}
            className="bg-surface-2 text-muted hover:text-cream text-sm px-3 py-1.5 rounded-btn transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {backfillStarting ? '啟動中...' : '補齊預覽'}
          </button>
          <button
            onClick={onOpenImport}
            className="bg-accent text-accent-ink hover:brightness-110 text-sm px-3 py-1.5 rounded-btn transition-colors"
          >
            匯入影片
          </button>
        </div>
      </div>

      {/* Row 2: tag filter chips */}
      {allTags.length > 0 && (
        <div className="flex flex-wrap gap-1.5 items-center">
          <button
            onClick={() => handleTagChip(null)}
            className={`text-xs px-2.5 py-1 rounded-btn transition-colors ${
              selectedTagIds.length === 0
                ? 'bg-accent text-accent-ink'
                : 'bg-surface-2 text-muted hover:text-cream'
            }`}
          >
            全部
          </button>
          {allTags.map((tag) => {
            const active = selectedTagIds.includes(tag.id)
            return (
              <button
                key={tag.id}
                onClick={() => handleTagChip(tag.id)}
                className={`text-xs px-2.5 py-1 rounded-btn transition-colors ${
                  active
                    ? 'bg-accent text-accent-ink'
                    : 'bg-surface-2 text-muted hover:text-cream'
                }`}
              >
                {tag.name}
              </button>
            )
          })}
        </div>
      )}

      {/* Row 3: batch bar */}
      {selected.length > 0 && (
        <div className="flex items-center gap-3 px-3 py-2 rounded-btn bg-accent/10 border border-accent/20">
          <span className="text-sm text-cream">已選取 {selected.length} 部</span>
          <button
            onClick={onBatchTag}
            className="text-xs text-accent hover:brightness-110 px-2 py-1 rounded-btn transition-colors"
          >
            批次加標籤
          </button>
          <button
            onClick={onBatchDelete}
            className="text-xs text-fav hover:brightness-110 px-2 py-1 rounded-btn transition-colors"
          >
            刪除
          </button>
          <button
            onClick={onClearSelection}
            className="text-xs text-muted hover:text-cream px-2 py-1 rounded-btn transition-colors ml-auto"
          >
            取消選取
          </button>
        </div>
      )}
    </div>
  )
}
