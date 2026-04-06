import { useState, useRef, useEffect } from 'react'
import { addVideoTag, removeVideoTag, createTag } from '../api/admin'
import type { Tag, TagWithCount } from '../types'

interface TagInputProps {
  videoId: string
  initialTags: Tag[]
  allTags: TagWithCount[]
  onTagsChange?: () => void
}

export default function TagInput({ videoId, initialTags, allTags, onTagsChange }: TagInputProps) {
  const [tags, setTags] = useState<Tag[]>(initialTags)
  const [input, setInput] = useState('')
  const [showDropdown, setShowDropdown] = useState(false)
  const [highlightIdx, setHighlightIdx] = useState(0)
  const inputRef = useRef<HTMLInputElement>(null)
  const containerRef = useRef<HTMLDivElement>(null)

  // Close dropdown on outside click
  useEffect(() => {
    function handleClickOutside(e: MouseEvent) {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setShowDropdown(false)
      }
    }
    document.addEventListener('mousedown', handleClickOutside)
    return () => document.removeEventListener('mousedown', handleClickOutside)
  }, [])

  const filtered = input.trim()
    ? allTags
        .filter((t) => t.name.toLowerCase().includes(input.toLowerCase().trim()))
        .filter((t) => !tags.some((vt) => vt.id === t.id))
        .slice(0, 10)
    : []

  const exactMatch = filtered.some((t) => t.name.toLowerCase() === input.toLowerCase().trim())
  const showCreate = input.trim().length > 0 && !exactMatch

  // Reset highlight when filtered list changes
  useEffect(() => {
    setHighlightIdx(0)
  }, [input])

  // Total selectable items: filtered tags + optional create
  const totalItems = filtered.length + (showCreate ? 1 : 0)

  async function handleSelectTag(tag: TagWithCount) {
    try {
      await addVideoTag(videoId, tag.id)
      setTags((prev) => [...prev, { id: tag.id, name: tag.name, category: tag.category }])
      setInput('')
      setShowDropdown(false)
      inputRef.current?.focus()
      onTagsChange?.()
    } catch { /* ignore */ }
  }

  async function handleCreateAndAdd() {
    const name = input.trim()
    if (!name) return
    try {
      const tag = await createTag(name, 'custom')
      await addVideoTag(videoId, tag.id)
      setTags((prev) => [...prev, { id: tag.id, name: tag.name, category: tag.category }])
      setInput('')
      setShowDropdown(false)
      inputRef.current?.focus()
      onTagsChange?.()
    } catch { /* ignore */ }
  }

  async function handleRemoveTag(tagId: number) {
    try {
      await removeVideoTag(videoId, tagId)
      setTags((prev) => prev.filter((t) => t.id !== tagId))
      onTagsChange?.()
    } catch { /* ignore */ }
  }

  function handleKeyDown(e: React.KeyboardEvent) {
    if (e.key === 'Escape') {
      setShowDropdown(false)
      return
    }

    if (e.key === 'Backspace' && input === '' && tags.length > 0) {
      handleRemoveTag(tags[tags.length - 1].id)
      return
    }

    if (!showDropdown || totalItems === 0) return

    if (e.key === 'ArrowDown') {
      e.preventDefault()
      setHighlightIdx((prev) => (prev + 1) % totalItems)
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      setHighlightIdx((prev) => (prev - 1 + totalItems) % totalItems)
    } else if (e.key === 'Enter') {
      e.preventDefault()
      if (highlightIdx < filtered.length) {
        handleSelectTag(filtered[highlightIdx])
      } else if (showCreate) {
        handleCreateAndAdd()
      }
    }
  }

  function handleInputChange(value: string) {
    setInput(value)
    setShowDropdown(value.trim().length > 0)
  }

  return (
    <div ref={containerRef} className="relative">
      <div
        className="flex flex-wrap gap-1 items-center bg-gray-800 border border-gray-700 rounded px-1.5 py-1 min-h-[32px] cursor-text"
        onClick={() => inputRef.current?.focus()}
      >
        {tags.map((tag) => (
          <span
            key={tag.id}
            className="inline-flex items-center gap-0.5 text-xs bg-indigo-600 text-white px-2 py-0.5 rounded-full"
          >
            {tag.name}
            <button
              onClick={(e) => { e.stopPropagation(); handleRemoveTag(tag.id) }}
              className="opacity-70 hover:opacity-100 ml-0.5"
            >
              &times;
            </button>
          </span>
        ))}
        <input
          ref={inputRef}
          value={input}
          onChange={(e) => handleInputChange(e.target.value)}
          onFocus={() => { if (input.trim()) setShowDropdown(true) }}
          onKeyDown={handleKeyDown}
          placeholder={tags.length === 0 ? '輸入標籤...' : ''}
          className="bg-transparent text-white text-xs outline-none flex-1 min-w-[60px] py-0.5"
        />
      </div>

      {showDropdown && totalItems > 0 && (
        <div className="absolute z-50 left-0 right-0 mt-1 bg-gray-800 border border-gray-700 rounded shadow-lg max-h-48 overflow-y-auto">
          {filtered.map((tag, idx) => (
            <button
              key={tag.id}
              onClick={() => handleSelectTag(tag)}
              className={`w-full text-left text-xs px-3 py-2 transition-colors ${
                idx === highlightIdx ? 'bg-gray-700 text-white' : 'text-gray-300 hover:bg-gray-700'
              }`}
            >
              {tag.name}
              <span className="text-gray-500 ml-1">({tag.video_count})</span>
            </button>
          ))}
          {showCreate && (
            <button
              onClick={handleCreateAndAdd}
              className={`w-full text-left text-xs px-3 py-2 border-t border-gray-700 transition-colors ${
                highlightIdx === filtered.length ? 'bg-gray-700 text-white' : 'text-gray-400 hover:bg-gray-700'
              }`}
            >
              <span className="text-indigo-400">+ 建立</span> &quot;{input.trim()}&quot;
            </button>
          )}
        </div>
      )}
    </div>
  )
}
