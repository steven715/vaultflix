import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { startStreamSource, type HlsInstance, type HlsPort, type StreamState } from './streamSource'
import { MAX_PREPARING_RETRIES, PREPARING_RETRY_DELAY_MS } from './hlsError'

class FakeMedia extends EventTarget {
  src = ''
  currentTime = 0
  loads = 0
  nativeHls = false
  load() {
    this.loads += 1
  }
  removeAttribute(name: string) {
    if (name === 'src') this.src = ''
  }
  canPlayType(type: string) {
    return this.nativeHls && type === 'application/vnd.apple.mpegurl' ? 'maybe' : ''
  }
  fire(type: string) {
    this.dispatchEvent(new Event(type))
  }
}

class FakeHls implements HlsInstance {
  sources: string[] = []
  media: unknown = null
  destroyed = false
  private handlers = new Map<string, (evt: unknown, data: never) => void>()
  on(event: string, cb: (evt: unknown, data: never) => void) {
    this.handlers.set(event, cb)
  }
  loadSource(url: string) {
    this.sources.push(url)
  }
  attachMedia(el: unknown) {
    this.media = el
  }
  destroy() {
    this.destroyed = true
  }
  emit(event: string, data: unknown = {}) {
    this.handlers.get(event)?.(event, data as never)
  }
}

function fakeHlsPort(supported: boolean) {
  const instances: FakeHls[] = []
  const port: HlsPort = {
    isSupported: () => supported,
    create: () => {
      const h = new FakeHls()
      instances.push(h)
      return h
    },
    events: { error: 'error', manifestParsed: 'parsed' },
  }
  return { port, instances }
}

const flush = () => new Promise((r) => setTimeout(r, 0))

function setup(opts: { playMode: 'direct' | 'remux'; startAt?: number; hlsSupported?: boolean; nativeHls?: boolean }) {
  const media = new FakeMedia()
  media.nativeHls = opts.nativeHls ?? false
  const { port, instances } = fakeHlsPort(opts.hlsSupported ?? true)
  const tokens = ['t1', 't2', 't3']
  const fetchToken = vi.fn(async () => {
    const t = tokens.shift()
    if (!t) throw new Error('no more tokens')
    return t
  })
  const states: StreamState[] = []
  const positions: Array<[number, string]> = []
  const source = startStreamSource({
    media,
    video: { id: 'v1', playMode: opts.playMode, streamUrl: '/api/videos/v1/stream' },
    startAt: opts.startAt ?? 0,
    fetchToken,
    hls: port,
    onState: (s) => states.push(s),
    onPositionSet: (seconds, reason) => positions.push([seconds, reason]),
  })
  const last = () => states[states.length - 1]
  return { media, instances, fetchToken, states, last, positions, source }
}

describe('startStreamSource: direct', () => {
  it('loads the stream with a fresh token', async () => {
    const { media, last } = setup({ playMode: 'direct' })
    await flush()
    expect(media.src).toBe('/api/videos/v1/stream?token=t1')
    expect(media.loads).toBe(1)
    media.fire('loadedmetadata')
    expect(last()).toEqual({ status: 'playing' })
  })

  it('resumes from startAt on the first load and reports it', async () => {
    const { media, positions } = setup({ playMode: 'direct', startAt: 300 })
    await flush()
    media.fire('loadedmetadata')
    expect(media.currentTime).toBe(300)
    expect(positions).toEqual([[300, 'resume']])
  })

  it('refreshes the token once on a media error and restores the position', async () => {
    const { media, fetchToken, positions } = setup({ playMode: 'direct' })
    await flush()
    media.fire('loadedmetadata')
    media.currentTime = 1234
    media.fire('error')
    await flush()
    expect(fetchToken).toHaveBeenCalledTimes(2)
    expect(media.src).toBe('/api/videos/v1/stream?token=t2')
    media.currentTime = 0
    media.fire('loadedmetadata')
    expect(media.currentTime).toBe(1234)
    expect(positions).toEqual([[1234, 'recovery']])
  })

  it('fails after a second error in the same episode', async () => {
    const { media, last } = setup({ playMode: 'direct' })
    await flush()
    media.fire('error')
    await flush()
    media.fire('error')
    expect(last()).toEqual({ status: 'failed', reason: 'media-error' })
  })

  it('a successful reload restores the one-refresh budget', async () => {
    const { media, fetchToken, last } = setup({ playMode: 'direct' })
    await flush()
    media.fire('error')
    await flush()
    media.fire('loadedmetadata') // recovered
    media.fire('error')
    await flush()
    expect(fetchToken).toHaveBeenCalledTimes(3)
    expect(last()).not.toEqual({ status: 'failed', reason: 'media-error' })
  })

  it('reports a missing first token separately from a failed refresh', async () => {
    const media = new FakeMedia()
    const states: StreamState[] = []
    startStreamSource({
      media,
      video: { id: 'v1', playMode: 'direct', streamUrl: '/s' },
      startAt: 0,
      fetchToken: async () => {
        throw new Error('403')
      },
      hls: fakeHlsPort(true).port,
      onState: (s) => states.push(s),
      onPositionSet: () => {},
    })
    await flush()
    expect(states[states.length - 1]).toEqual({ status: 'failed', reason: 'token-unavailable' })
  })

  it('reports a failed token refresh', async () => {
    const { media, fetchToken, last } = setup({ playMode: 'direct' })
    await flush()
    fetchToken.mockRejectedValueOnce(new Error('401'))
    media.fire('error')
    await flush()
    expect(last()).toEqual({ status: 'failed', reason: 'token-refresh-failed' })
  })
})

describe('startStreamSource: remux', () => {
  beforeEach(() => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
  })
  afterEach(() => {
    vi.useRealTimers()
  })

  it('plays through hls.js when MSE is available', async () => {
    const { media, instances } = setup({ playMode: 'remux' })
    await flush()
    expect(instances).toHaveLength(1)
    expect(instances[0].sources).toEqual(['/api/videos/v1/hls/index.m3u8?token=t1'])
    expect(instances[0].media).toBe(media)
    expect(media.src).toBe('')
  })

  it('polls while the Keyframe Index is being prepared, then plays', async () => {
    const { instances, last } = setup({ playMode: 'remux' })
    await flush()
    const hls = instances[0]
    hls.emit('error', { fatal: true, response: { code: 503 } })
    expect(last()).toEqual({ status: 'preparing' })
    await vi.advanceTimersByTimeAsync(PREPARING_RETRY_DELAY_MS)
    expect(hls.sources).toHaveLength(2)
    hls.emit('parsed')
    expect(last()).toEqual({ status: 'loading' })
  })

  it('gives up preparing after the retry limit', async () => {
    const { instances, last } = setup({ playMode: 'remux' })
    await flush()
    const hls = instances[0]
    for (let i = 0; i <= MAX_PREPARING_RETRIES; i++) {
      hls.emit('error', { fatal: true, response: { code: 503 } })
      await vi.advanceTimersByTimeAsync(PREPARING_RETRY_DELAY_MS)
    }
    expect(last()).toEqual({ status: 'failed', reason: 'preparing-timeout' })
  })

  it('treats any other fatal hls.js error as a stream load failure', async () => {
    const { instances, last } = setup({ playMode: 'remux' })
    await flush()
    instances[0].emit('error', { fatal: true, response: { code: 500 } })
    expect(last()).toEqual({ status: 'failed', reason: 'stream-load-failed' })
  })

  it('ignores non-fatal hls.js errors', async () => {
    const { instances, last } = setup({ playMode: 'remux' })
    await flush()
    instances[0].emit('error', { fatal: false })
    expect(last()).toEqual({ status: 'loading' })
  })

  it('falls back to native HLS without MSE', async () => {
    const { media, instances } = setup({ playMode: 'remux', hlsSupported: false, nativeHls: true })
    await flush()
    expect(instances).toHaveLength(0)
    expect(media.src).toBe('/api/videos/v1/hls/index.m3u8?token=t1')
  })

  it('reports an unsupported browser', async () => {
    const { last } = setup({ playMode: 'remux', hlsSupported: false })
    await flush()
    expect(last()).toEqual({ status: 'failed', reason: 'unsupported' })
  })

  it('dispose tears down hls.js and the pending retry', async () => {
    const { instances, source } = setup({ playMode: 'remux' })
    await flush()
    const hls = instances[0]
    hls.emit('error', { fatal: true, response: { code: 503 } })
    source.dispose()
    await vi.advanceTimersByTimeAsync(PREPARING_RETRY_DELAY_MS)
    expect(hls.destroyed).toBe(true)
    expect(hls.sources).toHaveLength(1)
  })
})

describe('startStreamSource: lifecycle', () => {
  it('stops reacting after dispose', async () => {
    const { media, fetchToken, source, states } = setup({ playMode: 'direct' })
    source.dispose()
    await flush()
    media.fire('error')
    await flush()
    expect(fetchToken).toHaveBeenCalledTimes(1)
    expect(media.src).toBe('')
    expect(states).toEqual([{ status: 'loading' }])
  })
})
