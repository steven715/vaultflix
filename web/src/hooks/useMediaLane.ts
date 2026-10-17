import { useCallback } from 'react'
import { useToast } from '../contexts/ToastContext'
import { useJob, type StartOutcome, type StartResult } from './useJob'
import { useJobCancel } from './useJobCancel'
import { backfillJobs, importJobs } from '../lib/jobKinds'
import type { JobView } from '../lib/jobProgress'
import type { BackfillKind } from '../types'
import { errorMessage } from '../api/errors'

/** One Job on the Media Lane, as an admin page drives it. */
export interface LaneJob<TArgs> {
  view: JobView | null
  /** Starting or running: this Job holds the lane. */
  busy: boolean
  /** Start the Job. A refused or failed start is toasted here. */
  start(args: TArgs): Promise<StartOutcome>
  /** Present only when the kind can be cancelled; a failed cancel is toasted. */
  cancel?: () => Promise<void>
  /** Forget the shown Job (e.g. closing its summary). */
  dismiss(): void
}

export interface MediaLane {
  import: LaneJob<string>
  backfill: LaneJob<BackfillKind>
  /** Why an Import cannot start now; null when it can. */
  importBlockedReason: string | null
  /** Why a Backfill cannot start now; null when it can. */
  backfillBlockedReason: string | null
}

export interface MediaLaneOptions {
  onImportFinished?: (view: JobView) => void
  onBackfillFinished?: (view: JobView) => void
}

// useMediaLane runs an admin page's side of the Media Lane (GLOSSARY.md「Job
// Lane」, ADR-0014): Import and Backfill Jobs share it, so while one this page
// knows of runs or is starting, the other cannot start and the page is told
// why. A Job started in another tab after the page opened is not seen until
// the server's 409, which is toasted with its reason. Start and cancel
// failures become toasts; what the page does when a Job ends stays with the
// page (the options).
export function useMediaLane({ onImportFinished, onBackfillFinished }: MediaLaneOptions = {}): MediaLane {
  const toast = useToast()
  const importJob = useJob(importJobs, { onFinished: onImportFinished })
  const backfillJob = useJob(backfillJobs, { onFinished: onBackfillFinished })

  const report = useCallback((result: StartResult, fallback: string): StartOutcome => {
    if (result.outcome === 'adopted') toast.error('已有任務進行中')
    // A busy lane's 409 names the Job holding it.
    else if (result.outcome === 'failed') toast.error(errorMessage(result.error, { fallback }))
    return result.outcome
  }, [toast])
  const startImportJob = importJob.start
  const startBackfillJob = backfillJob.start
  const startImport = useCallback(
    async (sourceId: string) => report(await startImportJob(sourceId), '匯入啟動失敗'),
    [report, startImportJob],
  )
  const startBackfill = useCallback(
    async (kind: BackfillKind) => report(await startBackfillJob(kind), '啟動補齊失敗'),
    [report, startBackfillJob],
  )
  const cancelImport = useJobCancel(importJob.cancel)
  const cancelBackfill = useJobCancel(backfillJob.cancel)

  const importBusy = importJob.starting || importJob.view?.phase === 'running'
  const backfillBusy = backfillJob.starting || backfillJob.view?.phase === 'running'

  return {
    import: { view: importJob.view, busy: importBusy, start: startImport, cancel: cancelImport, dismiss: importJob.dismiss },
    backfill: { view: backfillJob.view, busy: backfillBusy, start: startBackfill, cancel: cancelBackfill, dismiss: backfillJob.dismiss },
    importBlockedReason: backfillBusy ? '補齊執行中，完成或取消後再匯入' : importBusy ? '匯入執行中' : null,
    backfillBlockedReason: importBusy ? '匯入執行中，完成或取消後再補齊' : backfillBusy ? '補齊執行中' : null,
  }
}
