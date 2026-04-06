import { useState, useEffect } from 'react'
import { listTags } from '../api/tags'
import type { TagWithCount } from '../types'

interface TagSidebarProps {
  selectedTagIds: number[]
  onTagsChange: (tagIds: number[]) => void
}

export default function TagSidebar({ selectedTagIds, onTagsChange }: TagSidebarProps) {
  const [tags, setTags] = useState<TagWithCount[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    listTags()
      .then(setTags)
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [])

  function toggleTag(tagId: number) {
    if (selectedTagIds.includes(tagId)) {
      onTagsChange(selectedTagIds.filter((id) => id !== tagId))
    } else {
      onTagsChange([...selectedTagIds, tagId])
    }
  }

  const sortedTags = tags
    .filter((t) => t.video_count > 0)
    .sort((a, b) => b.video_count - a.video_count)

  if (loading) {
    return (
      <div className="w-56 shrink-0 p-4">
        <div className="text-gray-500 text-sm">載入標籤中...</div>
      </div>
    )
  }

  if (sortedTags.length === 0) return null

  return (
    <aside className="w-56 shrink-0 border-r border-gray-800 p-4 overflow-y-auto">
      <h2 className="text-sm font-semibold text-gray-400 uppercase tracking-wider mb-3">標籤篩選</h2>
      {selectedTagIds.length > 0 && (
        <button
          onClick={() => onTagsChange([])}
          className="text-xs text-indigo-400 hover:text-indigo-300 mb-3"
        >
          清除全部
        </button>
      )}
      <div className="space-y-0.5">
        {sortedTags.map((tag) => {
          const selected = selectedTagIds.includes(tag.id)
          return (
            <button
              key={tag.id}
              onClick={() => toggleTag(tag.id)}
              className={`w-full text-left text-sm px-2 py-1 rounded transition-colors flex justify-between items-center ${
                selected
                  ? 'bg-indigo-600/20 text-indigo-400'
                  : 'text-gray-400 hover:bg-gray-800 hover:text-gray-300'
              }`}
            >
              <span className="truncate">{tag.name}</span>
              <span className="text-xs text-gray-600 ml-1">{tag.video_count}</span>
            </button>
          )
        })}
      </div>
    </aside>
  )
}
