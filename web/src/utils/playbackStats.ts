// Pure, DOM-free helpers for the playback stats HUD.
//
// These functions take plain data (not live DOM objects) so the math and the
// "缺位元組 vs codec" classification can be unit-tested in a plain node
// environment. The usePlaybackStats hook is the only place that reads the DOM
// and adapts it into these inputs.

export type PlaybackPhase =
  | 'idle'
  | 'playing'
  | 'paused'
  | 'buffering' // 缺位元組（暫時 starved，可自行恢復）
  | 'network-error' // 缺位元組（MEDIA_ERR_NETWORK：bytes 拿不到的硬失敗）
  | 'decode-error' // codec 解不了碼（MEDIA_ERR_DECODE）
  | 'unsupported' // codec 解不了碼（MEDIA_ERR_SRC_NOT_SUPPORTED）

// The headline distinction the HUD must surface: is playback healthy, waiting
// for bytes, or unable to decode the bytes it already has?
export type StatsFamily = 'ok' | 'starved' | 'codec'

// The single contract between usePlaybackStats and <NetworkHud>.
export interface PlaybackStats {
  phase: PlaybackPhase
  family: StatsFamily
  bufferAheadSec: number | null // 主訊號；null = 尚無 buffered 資料
  downlinkMbps: number | null // null = 視窗內無可用樣本
  ttfbMs: number | null // 最近一筆 /stream range 的 TTFB
  rebufferCount: number // 本次播放累計 stall 次數
}

// A plain [start, end] buffered range in seconds — one entry of a TimeRanges
// object, decoupled from the DOM.
export interface TimeRange {
  start: number
  end: number
}

// A timestamped reading of the buffered leading edge, on the performance clock.
export interface BufferSample {
  bufferedEnd: number // seconds of media downloaded ahead (range end)
  atMs: number
}

// MediaError.code values, redeclared so this module stays DOM-free.
export const MEDIA_ERR_ABORTED = 1
export const MEDIA_ERR_NETWORK = 2
export const MEDIA_ERR_DECODE = 3
export const MEDIA_ERR_SRC_NOT_SUPPORTED = 4

// readyState at/above which playback can continue without stalling.
export const HAVE_FUTURE_DATA = 3

// Live playback signals sampled from the <video> element each display tick.
export interface PlaybackSignals {
  errorCode: number | null // video.error?.code ?? null
  paused: boolean
  stalled: boolean // a `waiting` fired without a later `playing`/`canplay`
  readyState: number
  currentTime: number
}

// bufferAhead returns how many seconds of media are buffered ahead of the
// current position. It finds the buffered range containing currentTime and
// returns range.end - currentTime. Returns 0 when the position sits in a gap or
// nothing is buffered (never negative).
export function bufferAhead(ranges: TimeRange[], currentTime: number): number {
  for (const r of ranges) {
    if (currentTime >= r.start && currentTime <= r.end) {
      return Math.max(0, r.end - currentTime)
    }
  }
  return 0
}

// bufferedEndAt returns the end (seconds) of the buffered range that currently
// contains currentTime — the leading edge of downloaded media — or null when the
// position is in a gap or nothing is buffered.
export function bufferedEndAt(ranges: TimeRange[], currentTime: number): number | null {
  for (const r of ranges) {
    if (currentTime >= r.start && currentTime <= r.end) return r.end
  }
  return null
}

// estimateDownlinkMbps estimates download throughput from how fast the buffered
// edge advances in wall-clock time, scaled by the stream's average bitrate
// (file_size × 8 / duration). This works for progressive streaming — one
// open-ended Range response — where Resource Timing yields no per-chunk samples.
//
// Returns null when it cannot measure: no prior reading, no elapsed time, or
// unknown bitrate. A buffered regression (seek / eviction) clamps growth to 0
// rather than reporting noise. At steady state the edge advances ~1s per real
// second, so the reading sits near the average bitrate; while buffering ahead it
// rises above it; while stalled it falls to ~0.
export function estimateDownlinkMbps(
  prev: BufferSample | null,
  cur: BufferSample,
  avgBitrateBps: number,
): number | null {
  if (!prev || avgBitrateBps <= 0) return null
  const dtSec = (cur.atMs - prev.atMs) / 1000
  if (dtSec <= 0) return null
  const grownSec = Math.max(0, cur.bufferedEnd - prev.bufferedEnd)
  return ((grownSec / dtSec) * avgBitrateBps) / 1e6
}

// classifyPhase maps live signals to a phase. Error codes take precedence: a
// decode/unsupported error is a codec problem and must never be reported as
// "waiting for bytes" (the core requirement).
export function classifyPhase(s: PlaybackSignals): PlaybackPhase {
  if (s.errorCode != null) {
    switch (s.errorCode) {
      case MEDIA_ERR_NETWORK:
        return 'network-error'
      case MEDIA_ERR_DECODE:
        return 'decode-error'
      case MEDIA_ERR_SRC_NOT_SUPPORTED:
        return 'unsupported'
      default:
        return 'idle' // MEDIA_ERR_ABORTED or unknown
    }
  }
  if (s.paused) return 'paused'
  if (s.currentTime <= 0 && s.readyState < HAVE_FUTURE_DATA) return 'idle'
  if (s.stalled || s.readyState < HAVE_FUTURE_DATA) return 'buffering'
  return 'playing'
}

// familyOf groups a phase into the headline diagnostic family that drives the
// HUD's colour and label.
export function familyOf(phase: PlaybackPhase): StatsFamily {
  switch (phase) {
    case 'buffering':
    case 'network-error':
      return 'starved'
    case 'decode-error':
    case 'unsupported':
      return 'codec'
    default:
      return 'ok'
  }
}

// SessionSummary is the terminal, session-cumulative playback quality report
// emitted once when a viewing ends. null means "not measured this session".
export interface SessionSummary {
  ttffMs: number | null
  watchedMs: number
  rebufferCount: number
  rebufferMs: number
  avgDownlinkMbps: number | null
  fatalErrorFamily: 'starved' | 'codec' | null
}

// fatalFamilyOf maps a phase to the fatal error family it represents, or null
// when the phase is not a fatal end state. Unlike familyOf, transient
// `buffering` is NOT fatal (it recovers) — only the hard MediaError phases are.
export function fatalFamilyOf(phase: PlaybackPhase): 'starved' | 'codec' | null {
  switch (phase) {
    case 'network-error':
      return 'starved'
    case 'decode-error':
    case 'unsupported':
      return 'codec'
    default:
      return null
  }
}
