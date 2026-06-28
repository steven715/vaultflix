import { useEffect, useState } from 'react'
import { listTags } from '../../api/tags'
import { createTag } from '../../api/admin'
import { useToast } from '../../contexts/ToastContext'
import { groupTagsByCategory } from '../../lib/tagGroups'
import type { TagWithCount } from '../../types'

const DOT: Record<string, string> = {
  genre: 'bg-accent', studio: 'bg-data-blue', actor: 'bg-data-purple', custom: 'bg-muted',
}

export default function TagManagePage() {
  const [tags, setTags] = useState<TagWithCount[]>([])
  const [showCreate, setShowCreate] = useState(false)
  const toast = useToast()

  function reload() {
    listTags().then(setTags).catch((err) => console.warn('failed to load tags', err))
  }

  useEffect(() => {
    let cancelled = false
    listTags()
      .then((t) => { if (!cancelled) setTags(t) })
      .catch((err) => console.warn('failed to load tags', err))
    return () => { cancelled = true }
  }, [])

  const groups = groupTagsByCategory(tags)

  return (
    <div className="p-7 max-w-[1100px]">
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="font-display font-bold text-xl tracking-tight text-cream">標籤與分類</h1>
          <p className="text-sm text-muted mt-0.5">
            <span className="font-mono">{tags.length}</span> 個標籤 ·{' '}
            <span className="font-mono">{groups.length}</span> 個分類
          </p>
        </div>
        <button
          onClick={() => setShowCreate(true)}
          className="bg-accent text-accent-ink text-sm font-medium px-4 py-2 rounded-btn hover:brightness-110"
        >
          新增標籤
        </button>
      </div>

      <div className="grid md:grid-cols-2 gap-4">
        {groups.map((g) => (
          <div key={g.category} className="bg-surface border border-border rounded-card p-5">
            <div className="flex items-center gap-2 mb-3">
              <span className={`w-2.5 h-2.5 rounded-full ${DOT[g.category] ?? 'bg-muted'}`} />
              <span className="font-display font-semibold text-cream">{g.label}</span>
              <span className="font-mono text-xs text-faint ml-auto">{g.tags.length}</span>
            </div>
            <div className="flex flex-wrap gap-2">
              {g.tags.map((tag) => (
                <span key={tag.id} className="bg-surface-2 rounded-pill px-2.5 py-1 text-sm text-cream">
                  {tag.name} <span className="font-mono text-faint">{tag.video_count}</span>
                </span>
              ))}
            </div>
          </div>
        ))}
      </div>

      {showCreate && (
        <CreateTagModal
          onClose={() => setShowCreate(false)}
          onCreated={() => { setShowCreate(false); reload(); toast.success('已新增標籤') }}
          onError={() => toast.error('新增失敗，請重試')}
        />
      )}
    </div>
  )
}

function CreateTagModal({
  onClose,
  onCreated,
  onError,
}: {
  onClose: () => void
  onCreated: () => void
  onError: () => void
}) {
  const [name, setName] = useState('')
  const [category, setCategory] = useState('genre')
  const [submitting, setSubmitting] = useState(false)

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    setSubmitting(true)
    try {
      await createTag(name.trim(), category)
      onCreated()
    } catch {
      onError()
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div
      className="fixed inset-0 flex items-center justify-center z-50 bg-[rgba(8,6,5,0.72)] backdrop-blur-[3px]"
      onClick={onClose}
    >
      <div
        className="bg-surface rounded-lg p-6 w-full max-w-md border border-border"
        onClick={(e) => e.stopPropagation()}
      >
        <h2 className="font-display font-semibold text-lg text-cream mb-4">新增標籤</h2>
        <form onSubmit={submit}>
          <label className="block text-sm text-muted mb-1">名稱</label>
          <input
            value={name}
            onChange={(e) => setName(e.target.value)}
            required
            disabled={submitting}
            className="w-full bg-surface-2 text-cream text-sm rounded-btn px-3 py-2 outline-none focus:ring-2 focus:ring-accent mb-3"
          />
          <label className="block text-sm text-muted mb-1">分類</label>
          <select
            value={category}
            onChange={(e) => setCategory(e.target.value)}
            disabled={submitting}
            className="w-full bg-surface-2 text-cream text-sm rounded-btn px-3 py-2 outline-none focus:ring-2 focus:ring-accent mb-4"
          >
            <option value="genre">類型</option>
            <option value="studio">工作室</option>
            <option value="actor">人物</option>
            <option value="custom">自訂</option>
          </select>
          <div className="flex justify-end gap-2">
            <button
              type="button"
              onClick={onClose}
              disabled={submitting}
              className="text-sm text-muted hover:text-cream px-3 py-1.5 rounded-btn"
            >
              取消
            </button>
            <button
              type="submit"
              disabled={submitting || !name.trim()}
              className="bg-accent text-accent-ink text-sm px-4 py-1.5 rounded-btn disabled:opacity-50 hover:brightness-110"
            >
              {submitting ? '建立中...' : '建立'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}
