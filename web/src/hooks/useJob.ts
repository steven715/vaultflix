import { useCallback, useEffect, useReducer, useRef, useState } from 'react'
import { useWS } from '../contexts/WebSocketContext'
import { reduceJob, type JobEvent, type JobKind, type JobView } from '../lib/jobProgress'
import type { WSMessage } from './useWebSocket'

/** How a start request ended: a new job, an already-running one taken over (409), or an error. */
export type StartOutcome = 'started' | 'adopted' | 'failed'

export interface StartResult {
  outcome: StartOutcome
  /** The API error, when the outcome is 'failed'. */
  error?: unknown
}

export interface UseJobResult<TArgs> {
  view: JobView | null
  starting: boolean
  start(args: TArgs): Promise<StartResult>
  /** Present only when the kind can be cancelled. */
  cancel?: () => Promise<void>
  /** Forget the shown job (e.g. closing its panel). */
  dismiss(): void
}

function eventFor<TArgs>(kind: JobKind<TArgs>, msg: WSMessage): JobEvent | null {
  switch (msg.type) {
    case kind.messages.progress:
      return kind.progressEvent(msg.payload)
    case kind.messages.complete: {
      const view = kind.finishedView(msg.payload)
      return view ? { type: 'finished', view } : null
    }
    case kind.messages.error: {
      const jobId = kind.errorJobId(msg.payload)
      return jobId ? { type: 'failed', jobId } : null
    }
    default:
      return null
  }
}

function isConflict(err: unknown): boolean {
  return (err as { response?: { status?: number } })?.response?.status === 409
}

// useJob runs one kind of job for an admin page: it takes over a job already
// running when the page opens, starts new ones (taking over the running job on
// 409), follows its WebSocket progress, and reports when it finishes. `kind`
// must be a stable object (see jobKinds.ts).
export function useJob<TArgs>(
  kind: JobKind<TArgs>,
  { onFinished }: { onFinished?: (view: JobView) => void } = {},
): UseJobResult<TArgs> {
  const [view, dispatch] = useReducer(reduceJob, null)
  const [starting, setStarting] = useState(false)
  const onFinishedRef = useRef(onFinished)
  useEffect(() => {
    onFinishedRef.current = onFinished
  }, [onFinished])

  // Take over a job that is already running when the page opens.
  useEffect(() => {
    let cancelled = false
    kind.active().then((v) => {
      if (!cancelled && v && v.phase === 'running') dispatch({ type: 'snapshot', view: v })
    }).catch((err) => console.warn('failed to restore active job', err))
    return () => { cancelled = true }
  }, [kind])

  // Follow WebSocket progress. Dispatching from a microtask keeps the state
  // update out of the synchronous effect body. Never cancel a queued event: a
  // newer message replacing lastMessage would otherwise drop this one, and a
  // dispatch after unmount is a no-op anyway. (Messages React batches away
  // before rendering are still missed; counts are absolute and the complete
  // message carries the final job, so the view self-corrects.)
  const { lastMessage } = useWS()
  useEffect(() => {
    if (!lastMessage) return
    const event = eventFor(kind, lastMessage)
    if (event) queueMicrotask(() => dispatch(event))
  }, [lastMessage, kind])

  // Report each job's end once.
  const notifiedRef = useRef<string | null>(null)
  useEffect(() => {
    if (!view || view.phase === 'running' || notifiedRef.current === view.id) return
    notifiedRef.current = view.id
    onFinishedRef.current?.(view)
  }, [view])

  const start = useCallback(async (args: TArgs): Promise<StartResult> => {
    setStarting(true)
    try {
      dispatch({ type: 'snapshot', view: await kind.start(args) })
      return { outcome: 'started' }
    } catch (err) {
      if (!isConflict(err)) return { outcome: 'failed', error: err }
      const running = await kind.active().catch(() => null)
      if (running) dispatch({ type: 'snapshot', view: running })
      return { outcome: 'adopted' }
    } finally {
      setStarting(false)
    }
  }, [kind])

  const viewId = view?.id
  const cancelJob = useCallback(async () => {
    if (viewId && kind.cancel) await kind.cancel(viewId)
  }, [kind, viewId])

  const dismiss = useCallback(() => dispatch({ type: 'reset' }), [])

  return { view, starting, start, cancel: kind.cancel ? cancelJob : undefined, dismiss }
}
