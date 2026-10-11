import { useState } from 'react'
import type { LibrarySortBy, SortOrder } from '../../lib/libraryParams'
import type { BackfillKind, TagWithCount } from '../../types'
import { parseTagIds, serializeTagIds, toggleTagId, toggleSort } from '../../lib/libraryParams'
import { backfillKindName, backfillMenu } from '../../lib/jobLabels'

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
  /** Why Backfill cannot start now (e.g. an Import holds the Media Lane); null when it can. */
  backfillDisabledReason: string | null
  onSort: (params: { sort_by: string; sort_order: string }) => void
  onTagFilter: (tag_ids: string) => void
  onStartBackfill: (kind: BackfillKind) => void
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
  backfillDisabledReason,
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
          <BackfillMenu disabledReason={backfillDisabledReason} onStart={onStartBackfill} />
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

// BackfillMenu is the 補齊 button: it opens a menu of the Backfill kinds.
function BackfillMenu({ disabledReason, onStart }: { disabledReason: string | null; onStart: (kind: BackfillKind) => void }) {
  const [open, setOpen] = useState(false)
  return (
    <div
      className="relative"
      onBlur={(e) => { if (!e.currentTarget.contains(e.relatedTarget)) setOpen(false) }}
      onKeyDown={(e) => { if (e.key === 'Escape') setOpen(false) }}
    >
      <button
        onClick={() => setOpen(!open)}
        disabled={disabledReason !== null}
        title={disabledReason ?? undefined}
        aria-haspopup="menu"
        aria-expanded={open}
        className="bg-surface-2 text-muted hover:text-cream text-sm px-3 py-1.5 rounded-btn transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
      >
        補齊 ▾
      </button>
      {open && (
        <div role="menu" className="absolute right-0 mt-1 z-20 min-w-36 bg-surface border border-border rounded-btn py-1 shadow-lg">
          {backfillMenu.map((kind) => (
            <button
              key={kind}
              role="menuitem"
              onClick={() => { setOpen(false); onStart(kind) }}
              className="block w-full text-left text-sm text-muted hover:text-cream hover:bg-surface-2 px-3 py-1.5"
            >
              {backfillKindName(kind)}
            </button>
          ))}
        </div>
      )}
    </div>
  )
}
