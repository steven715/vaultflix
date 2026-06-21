import { describe, it, expect } from 'vitest'

import {
  bufferAhead,
  bufferedEndAt,
  classifyPhase,
  estimateDownlinkMbps,
  familyOf,
  type PlaybackPhase,
  type PlaybackSignals,
  type StatsFamily,
  type TimeRange,
} from './playbackStats'

describe('bufferAhead', () => {
  const cases: { name: string; ranges: TimeRange[]; ct: number; want: number }[] = [
    { name: 'mid-range', ranges: [{ start: 0, end: 10 }], ct: 3, want: 7 },
    { name: 'at range end (boundary)', ranges: [{ start: 0, end: 10 }], ct: 10, want: 0 },
    { name: 'at range start (boundary)', ranges: [{ start: 0, end: 10 }], ct: 0, want: 10 },
    {
      name: 'second of two ranges',
      ranges: [
        { start: 0, end: 5 },
        { start: 20, end: 30 },
      ],
      ct: 25,
      want: 5,
    },
    {
      name: 'in a gap between ranges -> 0',
      ranges: [
        { start: 0, end: 5 },
        { start: 20, end: 30 },
      ],
      ct: 10,
      want: 0,
    },
    { name: 'nothing buffered -> 0', ranges: [], ct: 4, want: 0 },
  ]

  for (const c of cases) {
    it(c.name, () => {
      expect(bufferAhead(c.ranges, c.ct)).toBeCloseTo(c.want, 5)
    })
  }
})

describe('bufferedEndAt', () => {
  it('returns the end of the range containing currentTime', () => {
    expect(bufferedEndAt([{ start: 0, end: 30 }], 12)).toBe(30)
  })
  it('returns the matching range end with multiple ranges', () => {
    expect(
      bufferedEndAt(
        [
          { start: 0, end: 5 },
          { start: 20, end: 48 },
        ],
        25,
      ),
    ).toBe(48)
  })
  it('returns null in a gap', () => {
    expect(
      bufferedEndAt(
        [
          { start: 0, end: 5 },
          { start: 20, end: 48 },
        ],
        10,
      ),
    ).toBeNull()
  })
  it('returns null when nothing is buffered', () => {
    expect(bufferedEndAt([], 3)).toBeNull()
  })
})

describe('estimateDownlinkMbps', () => {
  const AVG = 8_000_000 // 8 Mbps average bitrate

  it('returns null without a prior reading', () => {
    expect(estimateDownlinkMbps(null, { bufferedEnd: 10, atMs: 1000 }, AVG)).toBeNull()
  })
  it('returns null when bitrate is unknown', () => {
    const prev = { bufferedEnd: 10, atMs: 0 }
    expect(estimateDownlinkMbps(prev, { bufferedEnd: 11, atMs: 1000 }, 0)).toBeNull()
  })
  it('returns null with no elapsed time', () => {
    const prev = { bufferedEnd: 10, atMs: 1000 }
    expect(estimateDownlinkMbps(prev, { bufferedEnd: 11, atMs: 1000 }, AVG)).toBeNull()
  })
  it('steady state (~1s edge growth per real second) ≈ average bitrate', () => {
    const prev = { bufferedEnd: 10, atMs: 0 }
    expect(estimateDownlinkMbps(prev, { bufferedEnd: 11, atMs: 1000 }, AVG)).toBeCloseTo(8, 3)
  })
  it('buffering ahead (4s edge growth in 1s) reads above bitrate', () => {
    const prev = { bufferedEnd: 10, atMs: 0 }
    expect(estimateDownlinkMbps(prev, { bufferedEnd: 14, atMs: 1000 }, AVG)).toBeCloseTo(32, 3)
  })
  it('stalled (no edge growth) reads 0', () => {
    const prev = { bufferedEnd: 10, atMs: 0 }
    expect(estimateDownlinkMbps(prev, { bufferedEnd: 10, atMs: 1000 }, AVG)).toBeCloseTo(0, 6)
  })
  it('buffered regression (seek/eviction) clamps to 0, not negative', () => {
    const prev = { bufferedEnd: 50, atMs: 0 }
    expect(estimateDownlinkMbps(prev, { bufferedEnd: 5, atMs: 1000 }, AVG)).toBe(0)
  })
})

describe('classifyPhase + familyOf', () => {
  const base: PlaybackSignals = {
    errorCode: null,
    paused: false,
    stalled: false,
    readyState: 4,
    currentTime: 5,
  }
  const cases: {
    name: string
    sig: Partial<PlaybackSignals>
    phase: PlaybackPhase
    family: StatsFamily
  }[] = [
    { name: 'network error -> starved', sig: { errorCode: 2 }, phase: 'network-error', family: 'starved' },
    { name: 'decode error -> codec (NOT starved)', sig: { errorCode: 3 }, phase: 'decode-error', family: 'codec' },
    { name: 'unsupported -> codec', sig: { errorCode: 4 }, phase: 'unsupported', family: 'codec' },
    { name: 'aborted -> idle', sig: { errorCode: 1 }, phase: 'idle', family: 'ok' },
    { name: 'paused', sig: { paused: true }, phase: 'paused', family: 'ok' },
    { name: 'playing', sig: {}, phase: 'playing', family: 'ok' },
    { name: 'stalled while playing -> buffering', sig: { stalled: true }, phase: 'buffering', family: 'starved' },
    {
      name: 'low readyState while playing -> buffering',
      sig: { readyState: 2 },
      phase: 'buffering',
      family: 'starved',
    },
    {
      name: 'not started (ct 0, readyState 0) -> idle',
      sig: { currentTime: 0, readyState: 0 },
      phase: 'idle',
      family: 'ok',
    },
  ]

  for (const c of cases) {
    it(c.name, () => {
      const phase = classifyPhase({ ...base, ...c.sig })
      expect(phase).toBe(c.phase)
      expect(familyOf(phase)).toBe(c.family)
    })
  }

  it('error code takes precedence over paused/stalled', () => {
    const phase = classifyPhase({ ...base, errorCode: 3, paused: true, stalled: true })
    expect(phase).toBe('decode-error')
    expect(familyOf(phase)).toBe('codec')
  })
})
