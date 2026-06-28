import type { VideoWithTags, TagWithCount, MediaSource } from '../../types'
import ImportProgress from './ImportProgress'

type ImportState = 'idle' | 'importing' | 'completed' | 'failed'

/* ---------- Batch Tag Picker ---------- */

interface BatchTagPickerProps {
  selectedCount: number
  allTags: TagWithCount[]
  tagId: number
  onTagChange: (id: number) => void
  onApply: () => void
  onClose: () => void
}

export function BatchTagPicker({ selectedCount, allTags, tagId, onTagChange, onApply, onClose }: BatchTagPickerProps) {
  return (
    <div className="fixed inset-0 bg-[rgba(8,6,5,0.72)] backdrop-blur-[3px] flex items-center justify-center z-50" onClick={onClose}>
      <div className="bg-surface border border-border rounded-lg p-6 w-full max-w-sm" onClick={(e) => e.stopPropagation()}>
        <h2 className="font-display font-bold tracking-tight text-cream text-lg mb-3">批次加標籤</h2>
        <p className="text-sm text-muted mb-3">選擇要套用到 {selectedCount} 部影片的標籤：</p>
        <select
          value={tagId}
          onChange={(e) => onTagChange(Number(e.target.value))}
          className="w-full bg-surface-2 text-cream text-sm rounded-btn px-3 py-2 outline-none focus:ring-2 focus:ring-accent mb-4 border border-border"
        >
          {allTags.map((t) => <option key={t.id} value={t.id}>{t.name}</option>)}
        </select>
        <div className="flex justify-end gap-2">
          <button onClick={onClose} className="text-sm text-muted hover:text-cream px-3 py-1.5 rounded-btn">取消</button>
          <button onClick={onApply} className="bg-accent text-accent-ink hover:brightness-110 text-sm px-4 py-1.5 rounded-btn">套用</button>
        </div>
      </div>
    </div>
  )
}

/* ---------- Import Modal ---------- */

interface ImportModalProps {
  importState: ImportState
  mediaSources: MediaSource[]
  selectedSourceID: string
  currentJobId: string | null
  onSourceChange: (id: string) => void
  onClose: () => void
  onStartImport: () => void
  onImportComplete: () => void
  onResetImport: () => void
}

export function ImportModal({
  importState, mediaSources, selectedSourceID, currentJobId,
  onSourceChange, onClose, onStartImport, onImportComplete, onResetImport,
}: ImportModalProps) {
  return (
    <div className="fixed inset-0 bg-[rgba(8,6,5,0.72)] backdrop-blur-[3px] flex items-center justify-center z-50" onClick={() => importState !== 'importing' && onClose()}>
      <div className="bg-surface rounded-lg p-6 w-full max-w-md" onClick={(e) => e.stopPropagation()}>
        <h2 className="font-display font-bold tracking-tight text-cream text-lg mb-4">匯入影片</h2>
        {importState === 'idle' && (
          <>
            <label className="block text-sm text-muted mb-1">選擇媒體來源</label>
            {mediaSources.length === 0 ? (
              <p className="text-sm text-faint mb-4">沒有可用的媒體來源</p>
            ) : (
              <select value={selectedSourceID} onChange={(e) => onSourceChange(e.target.value)} className="w-full bg-surface-2 text-cream text-sm rounded-btn px-3 py-2 outline-none focus:ring-2 focus:ring-accent mb-4 border border-border">
                {mediaSources.map((s) => <option key={s.id} value={s.id}>{s.label} ({s.mount_path})</option>)}
              </select>
            )}
            <div className="flex justify-end gap-2">
              <button onClick={onClose} className="text-sm text-muted hover:text-cream px-3 py-1.5 rounded-btn">取消</button>
              <button onClick={onStartImport} disabled={!selectedSourceID || mediaSources.length === 0} className="bg-accent text-accent-ink hover:brightness-110 disabled:opacity-50 text-sm px-4 py-1.5 rounded-btn">開始匯入</button>
            </div>
          </>
        )}
        {(importState === 'importing' || importState === 'completed' || importState === 'failed') && currentJobId && (
          <>
            <ImportProgress jobId={currentJobId} onComplete={onImportComplete} />
            {importState !== 'importing' && (
              <div className="flex justify-end gap-2 mt-3">
                <button onClick={() => { onResetImport(); onClose() }} className="text-sm text-muted hover:text-cream px-3 py-1.5 rounded-btn">關閉</button>
                <button onClick={onResetImport} className="bg-accent text-accent-ink hover:brightness-110 text-sm px-4 py-1.5 rounded-btn">重新匯入</button>
              </div>
            )}
            {importState === 'importing' && <p className="text-xs text-faint text-center mt-3">匯入進行中，請勿關閉此視窗...</p>}
          </>
        )}
      </div>
    </div>
  )
}

/* ---------- Edit Modal ---------- */

interface EditModalProps {
  title: string
  desc: string
  saving: boolean
  onTitleChange: (v: string) => void
  onDescChange: (v: string) => void
  onSave: () => void
  onClose: () => void
}

export function EditModal({ title, desc, saving, onTitleChange, onDescChange, onSave, onClose }: EditModalProps) {
  return (
    <div className="fixed inset-0 bg-[rgba(8,6,5,0.72)] backdrop-blur-[3px] flex items-center justify-center z-50" onClick={() => !saving && onClose()}>
      <div className="bg-surface rounded-lg p-6 w-full max-w-md" onClick={(e) => e.stopPropagation()}>
        <h2 className="font-display font-bold tracking-tight text-cream text-lg mb-4">編輯影片</h2>
        <label className="block text-sm text-muted mb-1">標題</label>
        <input value={title} onChange={(e) => onTitleChange(e.target.value)} className="w-full bg-surface-2 text-cream text-sm rounded-btn px-3 py-2 outline-none focus:ring-2 focus:ring-accent mb-3 border border-border" disabled={saving} />
        <label className="block text-sm text-muted mb-1">描述</label>
        <textarea value={desc} onChange={(e) => onDescChange(e.target.value)} rows={3} className="w-full bg-surface-2 text-cream text-sm rounded-btn px-3 py-2 outline-none focus:ring-2 focus:ring-accent mb-4 resize-none border border-border" disabled={saving} />
        <div className="flex justify-end gap-2">
          <button onClick={onClose} disabled={saving} className="text-sm text-muted hover:text-cream px-3 py-1.5 rounded-btn">取消</button>
          <button onClick={onSave} disabled={saving || !title.trim()} className="bg-accent text-accent-ink hover:brightness-110 disabled:opacity-50 text-sm px-4 py-1.5 rounded-btn">{saving ? '儲存中...' : '儲存'}</button>
        </div>
      </div>
    </div>
  )
}

/* ---------- Delete Confirm ---------- */

interface DeleteConfirmProps {
  video: VideoWithTags
  onConfirm: () => void
  onClose: () => void
}

export function DeleteConfirm({ video, onConfirm, onClose }: DeleteConfirmProps) {
  return (
    <div className="fixed inset-0 bg-[rgba(8,6,5,0.72)] backdrop-blur-[3px] flex items-center justify-center z-50" onClick={onClose}>
      <div className="bg-surface rounded-lg p-6 w-full max-w-sm" onClick={(e) => e.stopPropagation()}>
        <h2 className="font-display font-bold tracking-tight text-cream text-lg mb-2">確認刪除</h2>
        <p className="text-sm text-muted mb-4">確定要刪除「<span className="text-cream">{video.title}</span>」嗎？此操作無法復原。</p>
        <div className="flex justify-end gap-2">
          <button onClick={onClose} className="text-sm text-muted hover:text-cream px-3 py-1.5 rounded-btn">取消</button>
          <button onClick={onConfirm} className="bg-fav text-cream hover:brightness-110 text-sm px-4 py-1.5 rounded-btn">刪除</button>
        </div>
      </div>
    </div>
  )
}
