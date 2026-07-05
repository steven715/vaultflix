import { useCallback, useEffect, useRef, useState } from 'react'
import type { RefObject } from 'react'

import {
  bufferAhead,
  bufferedEndAt,
  classifyPhase,
  estimateDownlinkMbps,
  familyOf,
  fatalFamilyOf,
  type BufferSample,
  type PlaybackStats,
  type SessionSummary,
  type TimeRange,
} from '../utils/playbackStats'

const PUBLISH_INTERVAL_MS = 500 // ~2Hz display cadence — the only setState source
const RTT_FRESH_MS = 10_000 // RTT shows "—" if no stream request within this window
const MBPS_EWMA_ALPHA = 0.4 // smoothing for the buffered-growth throughput estimate

const INITIAL_STATS: PlaybackStats = {
  phase: 'idle',
  family: 'ok',
  bufferAheadSec: null,
  downlinkMbps: null,
  ttfbMs: null,
  rebufferCount: 0,
}

// usePlaybackStats observes a <video> element and publishes a PlaybackStats
// snapshot at a fixed display cadence.
//
// Download throughput is estimated from how fast video.buffered advances scaled
// by `avgBitrateBps` (file_size × 8 / duration), because this backend streams
// each seek as one open-ended Range response and Resource Timing therefore
// yields no per-chunk samples during steady playback. RTT is the TTFB of the
// most recent stream request (it refreshes around seeks / initial load) and
// falls back to null once stale.
//
// All counters and rolling state live in refs; the only setState happens on the
// publish interval, so high-frequency media/network events never trigger a React
// re-render (CLAUDE.md 前端規範：計數用 useRef、只在顯示頻率 setState).
//
// `streamPath` is the same-origin path of the stream URL (e.g.
// "/api/videos/abc/stream"); Resource Timing entries whose name contains it feed
// the TTFB readout. Pass null while it is unknown.
export function usePlaybackStats(
  videoRef: RefObject<HTMLVideoElement | null>,
  streamPath: string | null,
  avgBitrateBps: number | null,
): { stats: PlaybackStats; getSessionSummary: () => SessionSummary } {
  const [stats, setStats] = useState<PlaybackStats>(INITIAL_STATS)

  const rebufferRef = useRef(0)
  const stalledRef = useRef(false)
  const ttfbRef = useRef<number | null>(null)
  const ttfbAtRef = useRef<number | null>(null) // perf time of the latest TTFB
  const prevBufRef = useRef<BufferSample | null>(null)
  const mbpsRef = useRef<number | null>(null) // EWMA-smoothed throughput

  const playStartRef = useRef<number | null>(null) // perf time of first play intent
  const ttffRef = useRef<number | null>(null) // first-frame latency, set once
  const watchedMsRef = useRef(0) // wall-clock time spent in `playing`
  const stallStartRef = useRef<number | null>(null) // perf time the current stall began
  const rebufferMsRef = useRef(0) // cumulative stall duration
  const mbpsSumRef = useRef(0)
  const mbpsCountRef = useRef(0)
  const fatalFamilyRef = useRef<'starved' | 'codec' | null>(null)
  const lastTickRef = useRef<number | null>(null) // perf time of previous publish

  const getSessionSummary = useCallback(
    (): SessionSummary => ({
      ttffMs: ttffRef.current,
      watchedMs: Math.round(watchedMsRef.current),
      rebufferCount: rebufferRef.current,
      rebufferMs: Math.round(rebufferMsRef.current),
      avgDownlinkMbps: mbpsCountRef.current > 0 ? mbpsSumRef.current / mbpsCountRef.current : null,
      fatalErrorFamily: fatalFamilyRef.current,
    }),
    [],
  )

  useEffect(() => {
    // New stream target (a new video) — reset per-session counters. A
    // token-refresh reload keeps the same streamPath, so this effect does not
    // re-run and the counters survive it.
    rebufferRef.current = 0
    stalledRef.current = false
    ttfbRef.current = null
    ttfbAtRef.current = null
    prevBufRef.current = null
    mbpsRef.current = null
    playStartRef.current = null
    ttffRef.current = null
    watchedMsRef.current = 0
    stallStartRef.current = null
    rebufferMsRef.current = 0
    mbpsSumRef.current = 0
    mbpsCountRef.current = 0
    fatalFamilyRef.current = null
    lastTickRef.current = null

    // Listeners are bound lazily to whichever <video> is currently mounted: the
    // element can appear *after* this effect runs (PlayerPage sets `video`
    // before `loading` flips false, so streamPath turns non-null one render
    // before the <video> mounts). The publish tick rebinds on first sight, so
    // the HUD works regardless of mount ordering — and keying the interval on
    // streamPath alone means we never depend on videoRef.current at effect time.
    let boundEl: HTMLVideoElement | null = null
    const onWaiting = () => {
      stalledRef.current = true
      // Count genuine rebuffers, not seek-induced waiting.
      if (boundEl && boundEl.currentTime > 0 && !boundEl.seeking) {
        rebufferRef.current += 1
        stallStartRef.current = performance.now()
      }
    }
    const onResume = () => {
      stalledRef.current = false
      if (stallStartRef.current != null) {
        rebufferMsRef.current += performance.now() - stallStartRef.current
        stallStartRef.current = null
      }
    }
    const onPlay = () => {
      if (playStartRef.current == null) playStartRef.current = performance.now()
    }
    const bind = (el: HTMLVideoElement) => {
      boundEl = el
      el.addEventListener('waiting', onWaiting)
      el.addEventListener('playing', onResume)
      el.addEventListener('canplay', onResume)
      el.addEventListener('play', onPlay)
    }
    const unbind = () => {
      if (!boundEl) return
      boundEl.removeEventListener('waiting', onWaiting)
      boundEl.removeEventListener('playing', onResume)
      boundEl.removeEventListener('canplay', onResume)
      boundEl.removeEventListener('play', onPlay)
      boundEl = null
    }

    // --- Resource Timing: latest stream TTFB only (element-independent) ---
    let observer: PerformanceObserver | null = null
    if (streamPath && typeof PerformanceObserver !== 'undefined') {
      observer = new PerformanceObserver((list) => {
        for (const entry of list.getEntries() as PerformanceResourceTiming[]) {
          if (!entry.name.includes(streamPath)) continue
          if (entry.responseStart > 0 && entry.requestStart > 0) {
            ttfbRef.current = entry.responseStart - entry.requestStart
            ttfbAtRef.current = entry.responseEnd
          }
        }
      })
      try {
        observer.observe({ type: 'resource', buffered: true })
      } catch {
        observer = null
      }
    }

    // --- single publish point: read the live element + refs, emit one snapshot ---
    const publish = () => {
      const el = videoRef.current
      // (Re)bind listeners when the element first appears or is swapped out.
      if (el !== boundEl) {
        unbind()
        if (el) bind(el)
      }
      if (!el) {
        setStats(INITIAL_STATS)
        return
      }

      const now = performance.now()
      const ranges: TimeRange[] = []
      for (let i = 0; i < el.buffered.length; i++) {
        ranges.push({ start: el.buffered.start(i), end: el.buffered.end(i) })
      }

      // Throughput from buffered-edge growth × average bitrate, EWMA-smoothed.
      const curEnd = bufferedEndAt(ranges, el.currentTime)
      if (curEnd != null) {
        const sample: BufferSample = { bufferedEnd: curEnd, atMs: now }
        const inst = estimateDownlinkMbps(prevBufRef.current, sample, avgBitrateBps ?? 0)
        prevBufRef.current = sample
        if (inst != null) {
          mbpsRef.current =
            mbpsRef.current == null
              ? inst
              : MBPS_EWMA_ALPHA * inst + (1 - MBPS_EWMA_ALPHA) * mbpsRef.current
          mbpsSumRef.current += inst
          mbpsCountRef.current += 1
        }
      }

      const ttfb =
        ttfbAtRef.current != null && now - ttfbAtRef.current <= RTT_FRESH_MS
          ? ttfbRef.current
          : null

      const phase = classifyPhase({
        errorCode: el.error?.code ?? null,
        paused: el.paused,
        stalled: stalledRef.current,
        readyState: el.readyState,
        currentTime: el.currentTime,
      })

      // Watched wall-clock: accumulate time spent actually playing.
      if (lastTickRef.current != null && phase === 'playing') {
        watchedMsRef.current += now - lastTickRef.current
      }
      lastTickRef.current = now

      // TTFF: first advancing frame after a play intent, recorded once.
      if (ttffRef.current == null && playStartRef.current != null && el.currentTime > 0) {
        ttffRef.current = now - playStartRef.current
      }

      // Fatal end state (hard MediaError), sticky once seen.
      const fatal = fatalFamilyOf(phase)
      if (fatal) fatalFamilyRef.current = fatal

      setStats({
        phase,
        family: familyOf(phase),
        bufferAheadSec: ranges.length ? bufferAhead(ranges, el.currentTime) : null,
        downlinkMbps: mbpsRef.current,
        ttfbMs: ttfb,
        rebufferCount: rebufferRef.current,
      })
    }

    const intervalID = window.setInterval(publish, PUBLISH_INTERVAL_MS)
    publish() // immediate first paint

    return () => {
      window.clearInterval(intervalID)
      unbind()
      observer?.disconnect()
    }
  }, [videoRef, streamPath, avgBitrateBps])

  return { stats, getSessionSummary }
}
