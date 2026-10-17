import { useState } from 'react'
import { updateVideo } from '../../api/admin'
import { useToast } from '../../contexts/ToastContext'
import type { Video } from '../../types'

interface EditVideoDialogProps {
  video: Pick<Video, 'id' | 'title' | 'description'>
  /** The Video as saved (without its Tags); the page shows it in place. */
  onSaved: (updated: Video) => void
  onClose: () => void
}

// EditVideoDialog edits a Video's title and description and saves them; a
// failed save says so and keeps the dialog open.
export default function EditVideoDialog({ video, onSaved, onClose }: EditVideoDialogProps) {
  const toast = useToast()
  const [title, setTitle] = useState(video.title)
  const [desc, setDesc] = useState(video.description)
  const [saving, setSaving] = useState(false)

  async function save() {
    setSaving(true)
    try {
      onSaved(await updateVideo(video.id, { title, description: desc }))
      toast.success('已儲存')
    } catch {
      toast.error('儲存失敗，請重試')
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="fixed inset-0 bg-[rgba(8,6,5,0.72)] backdrop-blur-[3px] flex items-center justify-center z-50" onClick={() => !saving && onClose()}>
      <div role="dialog" aria-modal="true" aria-label="編輯影片" className="bg-surface rounded-lg p-6 w-full max-w-md" onClick={(e) => e.stopPropagation()}>
        <h2 className="font-display font-bold tracking-tight text-cream text-lg mb-4">編輯影片</h2>
        <label className="block text-sm text-muted mb-1">標題</label>
        <input value={title} onChange={(e) => setTitle(e.target.value)} className="w-full bg-surface-2 text-cream text-sm rounded-btn px-3 py-2 outline-none focus:ring-2 focus:ring-accent mb-3 border border-border" disabled={saving} />
        <label className="block text-sm text-muted mb-1">描述</label>
        <textarea value={desc} onChange={(e) => setDesc(e.target.value)} rows={3} className="w-full bg-surface-2 text-cream text-sm rounded-btn px-3 py-2 outline-none focus:ring-2 focus:ring-accent mb-4 resize-none border border-border" disabled={saving} />
        <div className="flex justify-end gap-2">
          <button onClick={onClose} disabled={saving} className="text-sm text-muted hover:text-cream px-3 py-1.5 rounded-btn">取消</button>
          <button onClick={() => { void save() }} disabled={saving || !title.trim()} className="bg-accent text-accent-ink hover:brightness-110 disabled:opacity-50 text-sm px-4 py-1.5 rounded-btn">{saving ? '儲存中...' : '儲存'}</button>
        </div>
      </div>
    </div>
  )
}
