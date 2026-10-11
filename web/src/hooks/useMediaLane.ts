import { useToast } from '../contexts/ToastContext'
import { useJob, type StartOutcome, type UseJobResult } from './useJob'
import { useJobCancel } from './useJobCancel'
import { backfillJobs, importJobs } from '../lib/jobKinds'
import type { JobView } from '../lib/jobProgress'
import type { BackfillKind } from '../types'
import { getErrorMessage } from '../utils/error'

/** One Job on the Media Lane, as an admin page drives it. */
export interface LaneJob<TArgs> {
  view: JobView | null
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
// Lane」, ADR-0014): Import and Backfill Jobs share it, so while one runs or
// is starting, the other cannot start, and the page is told why rather than
// drawing a 409. Start and cancel failures become toasts; what the page does
// when a Job ends stays with the page (the options).
export function useMediaLane({ onImportFinished, onBackfillFinished }: MediaLaneOptions = {}): MediaLane {
  const toast = useToast()
  const importJob = useJob(importJobs, { onFinished: onImportFinished })
  const backfillJob = useJob(backfillJobs, { onFinished: onBackfillFinished })
  const cancelImport = useJobCancel(importJob.cancel)
  const cancelBackfill = useJobCancel(backfillJob.cancel)

  function startWithToasts<TArgs>(job: UseJobResult<TArgs>, fallback: string) {
    return async (args: TArgs): Promise<StartOutcome> => {
      const { outcome, error } = await job.start(args)
      if (outcome === 'adopted') toast.error('已有任務進行中')
      // A busy lane's 409 names the Job holding it.
      else if (outcome === 'failed') toast.error(getErrorMessage(error, fallback))
      return outcome
    }
  }

  const importBusy = importJob.starting || importJob.view?.phase === 'running'
  const backfillBusy = backfillJob.starting || backfillJob.view?.phase === 'running'

  return {
    import: {
      view: importJob.view, start: startWithToasts(importJob, '匯入啟動失敗'),
      cancel: cancelImport, dismiss: importJob.dismiss,
    },
    backfill: {
      view: backfillJob.view, start: startWithToasts(backfillJob, '啟動補齊失敗'),
      cancel: cancelBackfill, dismiss: backfillJob.dismiss,
    },
    importBlockedReason: backfillBusy ? '補齊執行中，完成或取消後再匯入' : importBusy ? '匯入執行中' : null,
    backfillBlockedReason: importBusy ? '匯入執行中，完成或取消後再補齊' : backfillBusy ? '補齊執行中' : null,
  }
}
