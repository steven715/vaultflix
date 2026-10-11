import { useState } from 'react'
import type { JobView } from '../../lib/jobProgress'
import type { JobLabels } from '../../lib/jobLabels'

interface JobProgressPanelProps {
  view: JobView
  labels: JobLabels
  /** Shown while running when given; resolves once the request is sent. */
  onCancel?: () => Promise<void>
}

// JobProgressPanel shows one Import Job or Backfill Job: live counts while it
// runs, a summary with the failures once it ends.
export default function JobProgressPanel({ view, labels, onCancel }: JobProgressPanelProps) {
  const [cancelling, setCancelling] = useState(false)
  const [showErrors, setShowErrors] = useState(false)
  const { counts } = view

  async function handleCancel() {
    if (!onCancel || cancelling) return
    setCancelling(true)
    try {
      await onCancel()
    } finally {
      setCancelling(false)
    }
  }

  if (view.phase === 'running') {
    return (
      <div className="bg-surface-2/50 rounded-lg p-4 mt-3">
        <div className="flex justify-between items-start mb-3">
          <div className="text-sm text-accent font-medium">{labels.running}</div>
          {onCancel && (
            <button
              onClick={handleCancel}
              disabled={cancelling}
              className="text-xs px-2 py-1 rounded-btn bg-fav/15 text-fav hover:bg-fav/25 disabled:opacity-50"
            >
              {cancelling ? '取消中...' : '取消'}
            </button>
          )}
        </div>
        <div className="mb-3">
          <div className="flex justify-between text-sm text-muted mb-1">
            <span>進度</span>
            <span className="font-mono">{counts.processed} / {counts.total || '...'}</span>
          </div>
          <div className="w-full bg-surface-up rounded-full h-2">
            <div
              className="bg-accent h-2 rounded-full transition-all duration-300"
              style={{ width: counts.total > 0 ? `${(counts.processed / counts.total) * 100}%` : '0%' }}
            />
          </div>
        </div>
        {view.currentItem && <p className="text-xs text-faint mb-2 truncate">處理中: {view.currentItem}</p>}
        <div className="grid grid-cols-3 gap-2 text-sm">
          <Counter value={counts.succeeded} label="成功" tone="text-live" />
          <Counter value={counts.skipped} label={labels.skippedShort} tone="text-muted" />
          <Counter value={counts.failed} label="失敗" tone="text-fav" />
        </div>
      </div>
    )
  }

  const tone = view.phase === 'completed' ? 'text-live' : view.phase === 'failed' ? 'text-fav' : 'text-faint'
  return (
    <div className="bg-surface-2/50 rounded-lg p-4 mt-3">
      <div className={`text-sm mb-3 font-medium ${tone}`}>{labels.done[view.phase]}</div>
      <div className="space-y-1.5 text-sm mb-3">
        <SummaryRow label={labels.total} value={counts.total} tone="text-cream" />
        <SummaryRow label="已處理" value={counts.processed} tone="text-cream" />
        <SummaryRow label={labels.succeeded} value={counts.succeeded} tone="text-live" />
        <SummaryRow label={labels.skipped} value={counts.skipped} tone="text-muted" />
        <SummaryRow label="失敗" value={counts.failed} tone="text-fav" />
      </div>
      {view.errors.length > 0 && (
        <div>
          <button onClick={() => setShowErrors(!showErrors)} className="text-xs text-fav hover:text-fav/80 mb-1">
            {showErrors ? '收起' : '展開'}失敗詳情 ({view.errors.length})
          </button>
          {showErrors && (
            <div className="bg-surface border border-border rounded p-2 max-h-40 overflow-y-auto space-y-1">
              {view.errors.map((e, i) => (
                <div key={i} className="text-xs">
                  <span className="text-cream">{e.name}</span>
                  <span className="text-faint ml-1">— {e.error}</span>
                </div>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  )
}

function Counter({ value, label, tone }: { value: number; label: string; tone: string }) {
  return (
    <div className="text-center">
      <div className={`${tone} font-medium font-mono`}>{value}</div>
      <div className="text-faint text-xs">{label}</div>
    </div>
  )
}

function SummaryRow({ label, value, tone }: { label: string; value: number; tone: string }) {
  return (
    <div className={`flex justify-between ${tone}`}>
      <span>{label}</span>
      <span className="font-mono">{value}</span>
    </div>
  )
}
