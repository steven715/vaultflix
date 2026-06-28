import { Link } from 'react-router-dom'
import type { VideoWithTags, TagWithCount } from '../../types'
import type { LibrarySortBy, SortOrder } from '../../lib/libraryParams'
import { isAllSelected, toggleSelected, toggleSelectAll } from '../../lib/libraryParams'
import PosterThumb from './PosterThumb'
import TagInput from '../TagInput'
import { formatDuration, formatFileSize } from '../../utils/format'

interface LibraryTableProps {
  videos: VideoWithTags[]
  allTags: TagWithCount[]
  selected: string[]
  sortBy: LibrarySortBy
  sortOrder: SortOrder
  copyFeedback: Record<string, boolean>
  onSelect: (ids: string[]) => void
  onColSort: (col: LibrarySortBy) => void
  onCopyPath: (filename: string, id: string) => void
  onEdit: (video: VideoWithTags) => void
  onDelete: (video: VideoWithTags) => void
  onTagsChange: () => void
}

function colCls(col: LibrarySortBy, sortBy: LibrarySortBy) {
  return sortBy === col ? 'text-accent cursor-pointer' : 'text-muted cursor-pointer hover:text-cream'
}

function colArrow(col: LibrarySortBy, sortBy: LibrarySortBy, sortOrder: SortOrder) {
  return sortBy !== col ? '' : (sortOrder === 'asc' ? ' ↑' : ' ↓')
}

function is4KRes(resolution?: string) {
  if (!resolution) return false
  const r = resolution.toLowerCase()
  return r.includes('4k') || resolution.startsWith('38') || resolution.startsWith('4096')
}

export default function LibraryTable({
  videos, allTags, selected, sortBy, sortOrder, copyFeedback,
  onSelect, onColSort, onCopyPath, onEdit, onDelete, onTagsChange,
}: LibraryTableProps) {
  const pageIds = videos.map((v) => v.id)

  return (
    <div className="overflow-x-auto">
      <table className="w-full text-sm text-left">
        <thead className="bg-surface-2 text-muted border-b border-border">
          <tr>
            <th className="py-3 px-2 w-8">
              <input type="checkbox" checked={isAllSelected(selected, pageIds)} onChange={() => onSelect(toggleSelectAll(selected, pageIds))} className="accent-accent" />
            </th>
            <th className="py-3 px-2 w-16">縮圖</th>
            <th className="py-3 px-2 cursor-pointer" onClick={() => onColSort('title')}>
              <span className={colCls('title', sortBy)}>影片{colArrow('title', sortBy, sortOrder)}</span>
            </th>
            <th className="py-3 px-2 w-16 text-center">解析度</th>
            <th className="py-3 px-2 w-20 cursor-pointer" onClick={() => onColSort('duration_seconds')}>
              <span className={colCls('duration_seconds', sortBy)}>時長{colArrow('duration_seconds', sortBy, sortOrder)}</span>
            </th>
            <th className="py-3 px-2 w-20 cursor-pointer" onClick={() => onColSort('file_size_bytes')}>
              <span className={colCls('file_size_bytes', sortBy)}>大小{colArrow('file_size_bytes', sortBy, sortOrder)}</span>
            </th>
            <th className="py-3 px-2 w-44">標籤</th>
            <th className="py-3 px-2 w-24 cursor-pointer" onClick={() => onColSort('created_at')}>
              <span className={colCls('created_at', sortBy)}>建立{colArrow('created_at', sortBy, sortOrder)}</span>
            </th>
            <th className="py-3 px-2 w-24">操作</th>
          </tr>
        </thead>
        <tbody>
          {videos.map((video) => {
            const isSel = selected.includes(video.id)
            return (
              <tr key={video.id} className={`border-b border-border hover:bg-surface-2 transition-colors ${isSel ? 'bg-accent/[0.06]' : ''}`}>
                <td className="py-2 px-2">
                  <input type="checkbox" aria-label={`選取 ${video.title}`} checked={isSel} onChange={() => onSelect(toggleSelected(selected, video.id))} className="accent-accent" />
                </td>
                <td className="py-2 px-2">
                  <PosterThumb id={video.id} src={video.thumbnail_url} className="w-[52px]" />
                </td>
                <td className="py-2 px-2 max-w-xs">
                  <Link to={`/videos/${video.id}`} className="text-cream hover:text-accent transition-colors font-medium line-clamp-1">{video.title}</Link>
                  <p className="text-xs font-mono text-faint truncate">{video.original_filename}</p>
                </td>
                <td className="py-2 px-2 text-center">
                  <span className={`text-xs px-1.5 py-0.5 rounded ${is4KRes(video.resolution) ? 'bg-accent/15 text-accent' : 'bg-surface-2 text-muted'}`}>{video.resolution || '—'}</span>
                </td>
                <td className="py-2 px-2 font-mono text-muted text-xs">{formatDuration(video.duration_seconds)}</td>
                <td className="py-2 px-2 font-mono text-muted text-xs">{formatFileSize(video.file_size_bytes)}</td>
                <td className="py-2 px-2">
                  <TagInput videoId={video.id} initialTags={video.tags} allTags={allTags} onTagsChange={onTagsChange} />
                </td>
                <td className="py-2 px-2 font-mono text-muted text-xs whitespace-nowrap">{video.created_at ? video.created_at.slice(0, 10) : '—'}</td>
                <td className="py-2 px-2">
                  <div className="flex items-center gap-1.5">
                    <button onClick={() => onCopyPath(video.original_filename, video.id)} aria-label="複製路徑" className="text-xs text-muted hover:text-cream transition-colors" title="複製檔案路徑">
                      {copyFeedback[video.id]
                        ? <span className="text-live">✓</span>
                        : <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" /></svg>}
                    </button>
                    <button onClick={() => onEdit(video)} className="text-xs text-muted hover:text-cream transition-colors">編輯</button>
                    <button onClick={() => onDelete(video)} className="text-xs text-muted hover:text-fav transition-colors">刪除</button>
                  </div>
                </td>
              </tr>
            )
          })}
        </tbody>
      </table>
    </div>
  )
}
