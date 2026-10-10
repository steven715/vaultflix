import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { startPlaybackSession, type SessionSender } from './playbackSession'
import { MAX_HEARTBEAT_DELTA } from './heartbeat'
import type { SessionSummary } from '../utils/playbackStats'

class FakeMedia extends EventTarget {
  currentTime = 0
  at(seconds: number, event: string) {
    this.currentTime = seconds
    this.dispatchEvent(new Event(event))
  }
}

type Call = { kind: 'progress' | 'heartbeat' | 'telemetry'; leaving: boolean; body: Record<string, unknown> }

function recordingSender() {
  const calls: Call[] = []
  const sender: SessionSender = {
    saveProgress: (body, leaving) => calls.push({ kind: 'progress', leaving, body: { ...body } }),
    heartbeat: (body, leaving) => calls.push({ kind: 'heartbeat', leaving, body: { ...body } }),
    telemetry: (body) => calls.push({ kind: 'telemetry', leaving: true, body: { ...body } }),
  }
  return { sender, calls, of: (kind: Call['kind']) => calls.filter((c) => c.kind === kind) }
}

const played: SessionSummary = {
  ttffMs: 420, watchedMs: 30_000, rebufferCount: 1, rebufferMs: 800, avgDownlinkMbps: 55, fatalErrorFamily: null,
}
const neverPlayed: SessionSummary = {
  ttffMs: null, watchedMs: 0, rebufferCount: 0, rebufferMs: 0, avgDownlinkMbps: null, fatalErrorFamily: null,
}

function setup(summary: SessionSummary = played) {
  const media = new FakeMedia()
  const rec = recordingSender()
  const session = startPlaybackSession({
    media,
    video: { id: 'v1', playMode: 'direct' },
    sender: rec.sender,
    summary: () => summary,
    newSessionId: () => 'sess-1',
  })
  return { media, session, ...rec }
}

beforeEach(() => {
  vi.useFakeTimers()
  vi.setSystemTime(new Date('2026-10-11T00:00:00Z'))
})
afterEach(() => {
  vi.useRealTimers()
})

describe('Watch Progress', () => {
  it('reports at most once per 10 seconds and never the same second twice', () => {
    const { media, of } = setup()
    media.at(5, 'timeupdate')
    media.at(6, 'timeupdate') // within the throttle window
    vi.advanceTimersByTime(10_000)
    media.at(6.4, 'timeupdate') // same second as last report? no — 6 was never reported
    vi.advanceTimersByTime(10_000)
    media.at(6.9, 'timeupdate') // same second as the last report
    expect(of('progress').map((c) => c.body.seconds)).toEqual([5, 6])
  })

  it('reports on pause, skipping the first second and repeats', () => {
    const { media, of } = setup()
    media.at(0.5, 'pause')
    media.at(42.7, 'pause')
    media.at(42.2, 'pause')
    expect(of('progress')).toEqual([{ kind: 'progress', leaving: false, body: { videoId: 'v1', seconds: 42 } }])
  })

  it('sends the final position as a leaving request on end, unless already reported', () => {
    const { media, session, of } = setup()
    media.at(30, 'timeupdate')
    media.at(95.5, 'seeking')
    session.end()
    expect(of('progress').at(-1)).toEqual({ kind: 'progress', leaving: true, body: { videoId: 'v1', seconds: 95 } })

    const second = setup()
    second.media.at(30, 'timeupdate')
    second.session.end()
    expect(second.of('progress').filter((c) => c.leaving)).toEqual([])
  })
})

describe('Watch Session heartbeat', () => {
  it('accumulates real play time and flushes it every 15 seconds', () => {
    const { media, of } = setup()
    media.at(10, 'timeupdate')
    media.at(20, 'timeupdate')
    vi.advanceTimersByTime(15_000)
    expect(of('heartbeat')).toEqual([
      { kind: 'heartbeat', leaving: false, body: { session_id: 'sess-1', video_id: 'v1', played_delta: 20, position_seconds: 20 } },
    ])
    vi.advanceTimersByTime(15_000) // nothing new played
    expect(of('heartbeat')).toHaveLength(1)
  })

  it('caps a single sample at the heartbeat maximum', () => {
    const { media, session, of } = setup()
    media.at(100, 'timeupdate')
    session.end()
    expect(of('heartbeat')[0].body.played_delta).toBe(MAX_HEARTBEAT_DELTA)
  })

  it('does not count a seek as watch time', () => {
    const { media, session, of } = setup()
    media.at(10, 'timeupdate')
    media.at(5000, 'seeking')
    media.at(5003, 'timeupdate')
    session.end()
    expect(of('heartbeat')[0].body.played_delta).toBe(13)
  })

  it('does not count a position the Stream Source set (resume / recovery)', () => {
    const { media, session, of } = setup()
    session.positionSet(1800)
    media.at(1805, 'timeupdate')
    session.end()
    expect(of('heartbeat')[0].body.played_delta).toBe(5)
  })

  it('flushes the remainder as a leaving request on end', () => {
    const { media, session, of } = setup()
    media.at(7, 'timeupdate')
    session.end()
    expect(of('heartbeat')).toEqual([
      { kind: 'heartbeat', leaving: true, body: { session_id: 'sess-1', video_id: 'v1', played_delta: 7, position_seconds: 7 } },
    ])
  })
})

describe('Playback Telemetry', () => {
  it('sends the session summary once on end', () => {
    const { session, of } = setup()
    session.end()
    session.end()
    expect(of('telemetry')).toEqual([
      {
        kind: 'telemetry',
        leaving: true,
        body: {
          session_id: 'sess-1', video_id: 'v1', play_mode: 'direct', ttff_ms: 420, watched_ms: 30_000,
          rebuffer_count: 1, rebuffer_ms: 800, avg_downlink_mbps: 55, fatal_error_family: null,
        },
      },
    ])
  })

  it('sends nothing for a session that never played', () => {
    const { session, of } = setup(neverPlayed)
    session.end()
    expect(of('telemetry')).toEqual([])
  })
})

describe('lifecycle', () => {
  it('stops listening and flushing after end', () => {
    const { media, session, calls } = setup()
    session.end()
    const before = calls.length
    media.at(50, 'timeupdate')
    media.at(60, 'pause')
    vi.advanceTimersByTime(60_000)
    expect(calls.length).toBe(before)
  })
})
