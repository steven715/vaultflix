// Job progress：後台 Import Job 與 Backfill Job 的進度狀態。每種 job 的差異
// （API、WS 訊息類型、欄位名稱）由 jobKinds.ts 轉成這裡的共同形狀；這裡只管
// 「收到什麼事件、狀態怎麼變」。
//
// 計數一律以伺服器送來的數字為準（每則結果訊息都帶著 job 當下的計數），
// 不在前端累加——漏掉訊息或 REST 還原較晚回來都不會讓數字飄掉。

export type JobPhase = 'running' | 'completed' | 'failed' | 'cancelled'

export interface JobCounts {
  total: number
  processed: number
  succeeded: number
  skipped: number
  failed: number
}

export interface JobItemError {
  name: string
  error: string
}

export interface JobView {
  id: string
  phase: JobPhase
  counts: JobCounts
  /** The file or Video being processed right now ('' when idle). */
  currentItem: string
  errors: JobItemError[]
  /** What the job runs on, e.g. the Import's Media Source id. */
  subject?: string
}

export type JobEvent =
  | { type: 'snapshot'; view: JobView }
  | { type: 'item-started'; jobId: string; item: string }
  | { type: 'item-done'; jobId: string; counts: JobCounts; error?: JobItemError }
  | { type: 'finished'; view: JobView }
  | { type: 'failed'; jobId: string }
  | { type: 'reset' }

export function reduceJob(view: JobView | null, event: JobEvent): JobView | null {
  switch (event.type) {
    case 'reset':
      return null
    case 'snapshot':
      // A REST snapshot may arrive after live results moved the same job on.
      if (view && view.id === event.view.id && event.view.counts.processed < view.counts.processed) {
        return view
      }
      return event.view
    case 'finished':
      return view && view.id !== event.view.id ? view : event.view
  }
  if (!view || view.id !== event.jobId) return view
  switch (event.type) {
    case 'item-started':
      return { ...view, currentItem: event.item }
    case 'item-done':
      return {
        ...view,
        counts: event.counts,
        errors: event.error ? [...view.errors, event.error] : view.errors,
      }
    case 'failed':
      return { ...view, phase: 'failed', currentItem: '' }
  }
}

/**
 * One kind of job as the admin UI sees it (see jobKinds.ts). It turns the
 * kind's own API and WebSocket messages into JobViews and JobEvents.
 */
export interface JobKind<TArgs> {
  /** WebSocket message types this kind sends. */
  messages: { progress: string; complete: string; error: string }
  /** Start a job; resolves to its running view. Rejects with the API error. */
  start(args: TArgs): Promise<JobView>
  /** The latest job of this kind, if any. */
  active(): Promise<JobView | null>
  /** Present only when the kind can be cancelled. */
  cancel?(jobId: string): Promise<void>
  /** Map a progress payload to an event (item started or done). */
  progressEvent(payload: unknown): JobEvent | null
  /** Map a complete payload to the final view. */
  finishedView(payload: unknown): JobView | null
  /** The job id an error payload is about. */
  errorJobId(payload: unknown): string | null
}
