import { useState } from 'react'
import { addVideoTag } from '../../api/admin'
import { useToast } from '../../contexts/ToastContext'
import { runOnEach } from '../../lib/batch'
import type { TagWithCount } from '../../types'

/* ---------- Batch Tag Dialog ---------- */

interface BatchTagDialogProps {
  videoIds: string[]
  tags: TagWithCount[]
  /** The batch ran (some Videos may have failed); the page refreshes its list. */
  onDone: () => void
  onClose: () => void
}

// BatchTagDialog adds one Tag to every selected Video. It keeps going past a
// failure, then says how many Videos it could not tag.
export function BatchTagDialog({ videoIds, tags, onDone, onClose }: BatchTagDialogProps) {
  const toast = useToast()
  const [tagId, setTagId] = useState(tags[0]?.id ?? 0)
  // While the batch runs it can neither start again nor be closed.
  const [running, setRunning] = useState(false)
  const close = () => { if (!running) onClose() }

  async function apply() {
    setRunning(true)
    const ids = [...videoIds]
    const failed = await runOnEach(ids, (id) => addVideoTag(id, tagId))
    onClose()
    onDone()
    if (failed.length > 0) toast.error(`${ids.length} 部中有 ${failed.length} 部加標籤失敗`)
    else toast.success('批次標籤完成')
  }

  return (
    <div className="fixed inset-0 bg-[rgba(8,6,5,0.72)] backdrop-blur-[3px] flex items-center justify-center z-50" onClick={close}>
      <div role="dialog" aria-modal="true" aria-label="批次加標籤" className="bg-surface border border-border rounded-lg p-6 w-full max-w-sm" onClick={(e) => e.stopPropagation()}>
        <h2 className="font-display font-bold tracking-tight text-cream text-lg mb-3">批次加標籤</h2>
        <p className="text-sm text-muted mb-3">選擇要套用到 {videoIds.length} 部影片的標籤：</p>
        <select
          value={tagId}
          onChange={(e) => setTagId(Number(e.target.value))}
          disabled={running}
          className="w-full bg-surface-2 text-cream text-sm rounded-btn px-3 py-2 outline-none focus:ring-2 focus:ring-accent mb-4 border border-border"
        >
          {tags.map((t) => <option key={t.id} value={t.id}>{t.name}</option>)}
        </select>
        <div className="flex justify-end gap-2">
          <button onClick={close} disabled={running} className="text-sm text-muted hover:text-cream px-3 py-1.5 rounded-btn">取消</button>
          <button onClick={() => { void apply() }} disabled={running} className="bg-accent text-accent-ink hover:brightness-110 disabled:opacity-50 text-sm px-4 py-1.5 rounded-btn">{running ? '套用中...' : '套用'}</button>
        </div>
      </div>
    </div>
  )
}

/* ---------- Delete Confirm ---------- */

interface DeleteConfirmProps {
  /** What is deleted, as the question names it: 「a Video's title」, or 選取的 N 部影片. */
  what: string
  onConfirm: () => void
  onClose: () => void
}

export function DeleteConfirm({ what, onConfirm, onClose }: DeleteConfirmProps) {
  return (
    <div className="fixed inset-0 bg-[rgba(8,6,5,0.72)] backdrop-blur-[3px] flex items-center justify-center z-50" onClick={onClose}>
      <div role="dialog" aria-modal="true" aria-labelledby="delete-confirm-title" className="bg-surface rounded-lg p-6 w-full max-w-sm" onClick={(e) => e.stopPropagation()}>
        <h2 id="delete-confirm-title" className="font-display font-bold tracking-tight text-cream text-lg mb-2">確認刪除</h2>
        <p className="text-sm text-muted mb-4">確定要刪除<span className="text-cream">{what}</span>嗎？此操作無法復原。</p>
        <div className="flex justify-end gap-2">
          <button onClick={onClose} className="text-sm text-muted hover:text-cream px-3 py-1.5 rounded-btn">取消</button>
          <button onClick={onConfirm} className="bg-fav text-cream hover:brightness-110 text-sm px-4 py-1.5 rounded-btn">刪除</button>
        </div>
      </div>
    </div>
  )
}
