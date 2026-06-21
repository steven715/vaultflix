import { useEffect, useState, type ReactNode } from 'react'
import { listTags } from '../api/tags'
import type { TagWithCount } from '../types'

interface TagFilterBarProps {
  selectedTagIds: number[]
  onTagsChange: (tagIds: number[]) => void
}

// TagFilterBar is the horizontally scrollable chip row above the library grid.
// "全部" clears the selection; tags are multi-select toggles (solid accent when on).
export default function TagFilterBar({ selectedTagIds, onTagsChange }: TagFilterBarProps) {
  const [tags, setTags] = useState<TagWithCount[]>([])

  useEffect(() => {
    let cancelled = false
    listTags()
      .then((t) => {
        if (!cancelled) setTags(t)
      })
      .catch((err) => console.warn('failed to load tags', err))
    return () => {
      cancelled = true
    }
  }, [])

  function toggle(tagId: number) {
    if (selectedTagIds.includes(tagId)) {
      onTagsChange(selectedTagIds.filter((id) => id !== tagId))
    } else {
      onTagsChange([...selectedTagIds, tagId])
    }
  }

  const sorted = tags.filter((t) => t.video_count > 0).sort((a, b) => b.video_count - a.video_count)
  if (sorted.length === 0) return null

  const allActive = selectedTagIds.length === 0

  return (
    <div className="no-scrollbar -mx-4 flex gap-2 overflow-x-auto px-4 pb-1 sm:mx-0 sm:px-0">
      <Chip active={allActive} onClick={() => onTagsChange([])}>
        全部
      </Chip>
      {sorted.map((tag) => (
        <Chip key={tag.id} active={selectedTagIds.includes(tag.id)} onClick={() => toggle(tag.id)}>
          {tag.name}
          <span className={selectedTagIds.includes(tag.id) ? 'text-accent-ink/70' : 'text-faint'}>
            {' '}
            {tag.video_count}
          </span>
        </Chip>
      ))}
    </div>
  )
}

function Chip({
  active,
  onClick,
  children,
}: {
  active: boolean
  onClick: () => void
  children: ReactNode
}) {
  return (
    <button
      onClick={onClick}
      className={`shrink-0 whitespace-nowrap rounded-pill border px-3.5 py-1.5 text-sm font-medium transition-colors ${
        active
          ? 'border-accent bg-accent text-accent-ink'
          : 'border-border bg-surface text-muted hover:bg-surface-up hover:text-cream'
      }`}
    >
      {children}
    </button>
  )
}
