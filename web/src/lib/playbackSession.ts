// Playback Session：一次開啟播放頁觀看一部 Video、直到換片或離開。它擁有這段
// 期間對後端的所有回報——Watch Progress（節流、暫停、離開時）、Watch Session
// 心跳（累積真實觀看秒數，seek 與 Stream Source 移動位置都不算），以及結束時
// 的 Playback Telemetry。
//
// 不依賴 React：media 元素、送出請求的 sender 與品質摘要都由呼叫端注入。
import { clampDelta } from './heartbeat'
import type { SessionSummary } from '../utils/playbackStats'
import type { VideoDetail } from '../types'

export const PROGRESS_THROTTLE_MS = 10_000
export const HEARTBEAT_INTERVAL_MS = 15_000

/** The subset of HTMLVideoElement the Playback Session listens to. */
export interface PlaybackSessionMedia extends EventTarget {
  currentTime: number
}

export interface HeartbeatReport {
  session_id: string
  video_id: string
  played_delta: number
  position_seconds: number
}

export interface TelemetryReport {
  session_id: string
  video_id: string
  play_mode: VideoDetail['play_mode']
  ttff_ms: number | null
  watched_ms: number
  rebuffer_count: number
  rebuffer_ms: number
  avg_downlink_mbps: number | null
  fatal_error_family: string | null
}

/**
 * Where the session's reports go. `leaving` marks the final reports sent while
 * the page tears down; those must survive it (keepalive). Telemetry has no
 * flag: it is only ever sent at the end. Fire-and-forget: failures are the
 * sender's to log.
 */
export interface PlaybackSessionSender {
  saveProgress(report: { videoId: string; seconds: number }, leaving: boolean): void
  heartbeat(report: HeartbeatReport, leaving: boolean): void
  telemetry(report: TelemetryReport): void
}

export interface PlaybackSessionOptions {
  media: PlaybackSessionMedia
  video: { id: string; playMode: VideoDetail['play_mode'] }
  sender: PlaybackSessionSender
  /** The quality summary to report at the end (from usePlaybackStats). */
  summary: () => SessionSummary
  newSessionId?: () => string
}

export interface PlaybackSession {
  /** The Stream Source moved the position itself: it is not watch time. */
  positionSet(seconds: number): void
  /** Send the final reports and stop. Idempotent. */
  end(): void
}

export function startPlaybackSession(opts: PlaybackSessionOptions): PlaybackSession {
  const { media, video, sender } = opts
  const sessionId = (opts.newSessionId ?? (() => crypto.randomUUID()))()
  let ended = false
  let lastReportAt = 0
  let lastReportSeconds = -1
  let lastSample = 0 // the currentTime the next heartbeat delta is measured from
  let pendingDelta = 0 // play-seconds accumulated since the last heartbeat
  // Where playback last was, per the media events. The final reports use it,
  // not media.currentTime: the Stream Source's teardown may already have
  // reset the element when the session ends.
  let lastPosition = 0

  function reportProgress(at: number, { leaving, throttled }: { leaving: boolean; throttled: boolean }) {
    const seconds = Math.floor(at)
    if (seconds === lastReportSeconds) return
    const now = Date.now()
    if (throttled && now - lastReportAt < PROGRESS_THROTTLE_MS) return
    lastReportAt = now
    lastReportSeconds = seconds
    sender.saveProgress({ videoId: video.id, seconds }, leaving)
  }

  function flushHeartbeat(at: number, leaving: boolean) {
    if (pendingDelta <= 0) return
    const report = {
      session_id: sessionId,
      video_id: video.id,
      played_delta: pendingDelta,
      position_seconds: Math.floor(at),
    }
    pendingDelta = 0
    sender.heartbeat(report, leaving)
  }

  function sendTelemetry() {
    const s = opts.summary()
    if (s.ttffMs == null && s.watchedMs <= 0) return // never played
    sender.telemetry({
      session_id: sessionId,
      video_id: video.id,
      play_mode: video.playMode,
      ttff_ms: s.ttffMs,
      watched_ms: s.watchedMs,
      rebuffer_count: s.rebufferCount,
      rebuffer_ms: s.rebufferMs,
      avg_downlink_mbps: s.avgDownlinkMbps,
      fatal_error_family: s.fatalErrorFamily,
    })
  }

  const onTimeUpdate = () => {
    lastPosition = media.currentTime
    reportProgress(lastPosition, { leaving: false, throttled: true })
    pendingDelta += clampDelta(lastSample, lastPosition)
    lastSample = lastPosition
  }
  // A seek (scrubber drag / skip) is not playback: measure from the target.
  const onSeeking = () => {
    lastPosition = media.currentTime
    lastSample = lastPosition
  }
  const onPause = () => {
    lastPosition = media.currentTime
    if (lastPosition < 1) return
    reportProgress(lastPosition, { leaving: false, throttled: false })
  }

  media.addEventListener('timeupdate', onTimeUpdate)
  media.addEventListener('seeking', onSeeking)
  media.addEventListener('pause', onPause)
  const timer = setInterval(() => flushHeartbeat(media.currentTime, false), HEARTBEAT_INTERVAL_MS)

  return {
    positionSet(seconds) {
      if (ended) return
      lastPosition = seconds
      lastSample = seconds
    },
    end() {
      if (ended) return
      ended = true
      clearInterval(timer)
      media.removeEventListener('timeupdate', onTimeUpdate)
      media.removeEventListener('seeking', onSeeking)
      media.removeEventListener('pause', onPause)
      if (lastPosition >= 1) reportProgress(lastPosition, { leaving: true, throttled: false })
      flushHeartbeat(lastPosition, true)
      sendTelemetry()
    },
  }
}
