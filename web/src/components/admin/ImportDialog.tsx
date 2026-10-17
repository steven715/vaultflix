import { useState } from 'react'
import { listMediaSources } from '../../api/admin'
import ErrorBanner from '../ErrorBanner'
import JobProgressPanel from './JobProgressPanel'
import type { LaneJob } from '../../hooks/useMediaLane'
import { useRemoteResource } from '../../hooks/useRemoteResource'
import { importLabels } from '../../lib/jobLabels'

interface ImportDialogProps {
  /** The Media Lane's Import Job (useMediaLane().import). */
  job: LaneJob<string>
  /** Why an Import cannot start now (e.g. a Backfill holds the Media Lane); null when it can. */
  blockedReason: string | null
  /** The admin asked to import. */
  open: boolean
  onClose: () => void
}

// ImportDialog starts an Import of an enabled Media Source and shows its
// progress. It shows while the admin has it open, and also on its own while
// an Import Job is shown (one adopted from another tab, or a summary not yet
// closed).
export default function ImportDialog({ job, blockedReason, open, onClose }: ImportDialogProps) {
  if (!open && job.view === null) return null
  return <ImportDialogBody job={job} blockedReason={blockedReason} onClose={onClose} />
}

function ImportDialogBody({ job, blockedReason, onClose }: Omit<ImportDialogProps, 'open'>) {
  const sources = useRemoteResource(async () => (await listMediaSources()).filter((s) => s.enabled))
  const [chosen, setChosen] = useState('')
  // The chosen Media Source, unless a reload no longer lists it.
  const sourceId = sources.data?.some((s) => s.id === chosen) ? chosen : sources.data?.[0]?.id ?? ''
  const view = job.view
  const running = view?.phase === 'running'

  function close() {
    job.dismiss()
    onClose()
  }

  return (
    <div className="fixed inset-0 bg-[rgba(8,6,5,0.72)] backdrop-blur-[3px] flex items-center justify-center z-50" onClick={() => !running && close()}>
      <div role="dialog" aria-modal="true" aria-label="匯入影片" className="bg-surface rounded-lg p-6 w-full max-w-md" onClick={(e) => e.stopPropagation()}>
        <h2 className="font-display font-bold tracking-tight text-cream text-lg mb-4">匯入影片</h2>
        {!view && (
          <>
            <label className="block text-sm text-muted mb-1">選擇媒體來源</label>
            {sources.status === 'loading' ? (
              <p className="text-sm text-faint mb-4">載入中...</p>
            ) : sources.status === 'error' ? (
              <ErrorBanner message="無法載入媒體來源，請確認服務是否正常運作" onRetry={sources.retry} />
            ) : sources.data?.length === 0 ? (
              <p className="text-sm text-faint mb-4">沒有可用的媒體來源</p>
            ) : (
              <select value={sourceId} onChange={(e) => setChosen(e.target.value)} className="w-full bg-surface-2 text-cream text-sm rounded-btn px-3 py-2 outline-none focus:ring-2 focus:ring-accent mb-4 border border-border">
                {sources.data?.map((s) => <option key={s.id} value={s.id}>{s.label} ({s.mount_path})</option>)}
              </select>
            )}
            {blockedReason && <p className="text-xs text-faint mb-3">{blockedReason}</p>}
            <div className="flex justify-end gap-2">
              <button onClick={close} className="text-sm text-muted hover:text-cream px-3 py-1.5 rounded-btn">取消</button>
              <button onClick={() => { void job.start(sourceId) }} disabled={!sourceId || blockedReason !== null} className="bg-accent text-accent-ink hover:brightness-110 disabled:opacity-50 text-sm px-4 py-1.5 rounded-btn">開始匯入</button>
            </div>
          </>
        )}
        {view && (
          <>
            <JobProgressPanel view={view} labels={importLabels} onCancel={job.cancel} />
            {!running && (
              <div className="flex justify-end gap-2 mt-3">
                <button onClick={close} className="text-sm text-muted hover:text-cream px-3 py-1.5 rounded-btn">關閉</button>
                <button onClick={job.dismiss} className="bg-accent text-accent-ink hover:brightness-110 text-sm px-4 py-1.5 rounded-btn">重新匯入</button>
              </div>
            )}
            {running && <p className="text-xs text-faint text-center mt-3">匯入進行中，請勿關閉此視窗...</p>}
          </>
        )}
      </div>
    </div>
  )
}
