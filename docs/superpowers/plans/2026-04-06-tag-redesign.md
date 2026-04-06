# Tag 交互重新設計 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the multi-step tag editing UI in admin video management with a Jira-style autocomplete tag input, and simplify the sidebar from category-grouped to a flat list sorted by video count.

**Architecture:** Create a new `TagInput` component that handles autocomplete, tag creation, and removal in a single input. Modify `VideoManagePage` to use it instead of the existing tag management UI. Modify `TagSidebar` to render a flat list sorted by `video_count` descending.

**Tech Stack:** React 18 + TypeScript, existing API endpoints (no backend changes)

---

### Task 1: Create TagInput component

**Files:**
- Create: `web/src/components/TagInput.tsx`

- [ ] **Step 1: Create the TagInput component file**

```tsx
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
```

- [ ] **Step 2: Verify no TypeScript errors**

Run:
```bash
docker compose exec vaultflix-web sh -c "cd /app && npx tsc --noEmit" 2>&1 | tail -5
```

Note: The web container serves built files via nginx, so TypeScript checking needs to happen during build. Alternatively verify by rebuilding:
```bash
docker compose build vaultflix-web 2>&1 | tail -10
```
Expected: Build succeeds without errors.

- [ ] **Step 3: Commit**

```bash
git add web/src/components/TagInput.tsx
git commit -m "feat: add Jira-style TagInput autocomplete component"
```

---

### Task 2: Replace tag editing UI in VideoManagePage

**Files:**
- Modify: `web/src/pages/admin/VideoManagePage.tsx`

- [ ] **Step 1: Update imports — remove unused tag management imports, add TagInput**

Replace the import line:
```tsx
import { importVideos, updateVideo, deleteVideo, addVideoTag, removeVideoTag, createTag } from '../../api/admin'
```
with:
```tsx
import { importVideos, updateVideo, deleteVideo } from '../../api/admin'
```

Add the TagInput import after the existing component imports:
```tsx
import TagInput from '../../components/TagInput'
```

- [ ] **Step 2: Remove tag management state variables**

Remove these three lines (lines 47-49):
```tsx
  const [tagManageVideoId, setTagManageVideoId] = useState<string | null>(null)
  const [newTagName, setNewTagName] = useState('')
  const [newTagCategory, setNewTagCategory] = useState('custom')
```

- [ ] **Step 3: Remove tag handler functions**

Remove `handleAddTag`, `handleRemoveTag`, and `handleCreateTag` functions (lines 149-181):
```tsx
  // Tag handlers
  async function handleAddTag(videoId: string, tagId: number) { ... }
  async function handleRemoveTag(videoId: string, tagId: number) { ... }
  async function handleCreateTag() { ... }
```

- [ ] **Step 4: Replace the tag cell in the video table**

Replace the entire `<td>` for tags (lines 250-295) — the cell containing `flex flex-wrap gap-1 items-center` and the `tagManageVideoId` conditional — with:

```tsx
                    <td className="py-2 px-2">
                      <TagInput
                        videoId={video.id}
                        initialTags={video.tags}
                        allTags={allTags}
                        onTagsChange={() => {
                          listTags().then(setAllTags).catch(() => {})
                        }}
                      />
                    </td>
```

- [ ] **Step 5: Verify build succeeds**

```bash
docker compose build vaultflix-web 2>&1 | tail -10
```
Expected: Build succeeds.

- [ ] **Step 6: Commit**

```bash
git add web/src/pages/admin/VideoManagePage.tsx
git commit -m "refactor: replace tag editing UI with TagInput autocomplete in admin page"
```

---

### Task 3: Simplify TagSidebar to flat list sorted by video count

**Files:**
- Modify: `web/src/components/TagSidebar.tsx`

- [ ] **Step 1: Replace the component body**

Replace the entire content of `web/src/components/TagSidebar.tsx` with:

```tsx
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
```

- [ ] **Step 2: Verify build succeeds**

```bash
docker compose build vaultflix-web 2>&1 | tail -10
```
Expected: Build succeeds.

- [ ] **Step 3: Commit**

```bash
git add web/src/components/TagSidebar.tsx
git commit -m "refactor: simplify TagSidebar to flat list sorted by video count"
```

---

### Task 4: Rebuild, deploy, and verify in browser

**Files:** No code changes — deployment and manual verification.

- [ ] **Step 1: Rebuild and restart frontend**

```bash
docker compose build vaultflix-web && docker compose up -d --force-recreate vaultflix-web
```

Wait for healthy:
```bash
timeout 60 bash -c 'while true; do status=$(docker compose ps vaultflix-web --format "{{.Status}}"); if echo "$status" | grep -q "healthy"; then echo "healthy"; break; fi; sleep 3; done'
```

- [ ] **Step 2: Verify admin tag editing — navigate to admin page**

Navigate to `http://localhost:3000/admin`. In the video table, the tag column should show:
- Tags as indigo pill badges (not plain text)
- An inline input field for typing
- No "+" button, no dropdown, no category selector

- [ ] **Step 3: Verify admin tag editing — test autocomplete**

Click the input area next to the pills, type a few characters. A dropdown should appear showing:
- Matching existing tags (with video count)
- "建立" option at the bottom if no exact match

Select an existing tag → it should appear as a new pill.
Type a new tag name → press Enter → it should be created and added.
Click × on a pill → it should be removed.

- [ ] **Step 4: Verify sidebar — navigate to home page**

Navigate to `http://localhost:3000/`. The left sidebar should show:
- "標籤篩選" heading
- All tags in a flat list (no category group headings)
- Sorted by video count descending
- Tags with 0 videos should not appear

- [ ] **Step 5: Commit all remaining changes (if any)**

If any fixes were needed, commit them:
```bash
git add -A && git commit -m "fix: tag redesign adjustments from manual testing"
```
