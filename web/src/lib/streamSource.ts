// Stream Source：決定一部 Video 用哪條路徑播放、把它接到 <video> 上，並負責
// 播放中的恢復——Keyframe Index 準備中的輪詢、stream token 過期時換新並跳回原位、
// 初次載入時從 Watch Progress 繼續。頁面只看它回報的狀態。
//
// 不依賴 React：media 元素、hls.js 與取得 token 的函式都由呼叫端注入，
// 測試換成 fake 即可。
import { classifyHlsError, PREPARING_RETRY_DELAY_MS, type HlsErrorDataLike } from './hlsError'

export type StreamPlayMode = 'direct' | 'remux'

export type StreamFailure =
  | 'media-error' // <video> 換過一次 token 後仍出錯
  | 'stream-load-failed' // hls.js 的致命錯誤：非「準備中」，或換過 token 仍 401
  | 'preparing-timeout' // Keyframe Index 準備中，輪詢到上限仍未就緒
  | 'token-unavailable' // 初次取得 stream token 失敗
  | 'token-refresh-failed' // 出錯後換新 token 失敗
  | 'unsupported' // 瀏覽器既無 MSE 也無原生 HLS

/** Why the Stream Source moved the playback position itself. */
export type PositionReason = 'resume' | 'recovery'

export type StreamState =
  | { status: 'loading' }
  | { status: 'preparing' }
  | { status: 'playing' }
  | { status: 'failed'; reason: StreamFailure }

/** The subset of HTMLVideoElement the Stream Source drives. */
export interface StreamMedia extends EventTarget {
  src: string
  currentTime: number
  load(): void
  removeAttribute(name: string): void
  canPlayType(type: string): string
}

export interface HlsInstance {
  on(event: string, cb: (evt: unknown, data: never) => void): void
  loadSource(url: string): void
  attachMedia(media: never): void
  destroy(): void
}

/** hls.js as the Stream Source sees it; see hlsPort.ts for the real adapter. */
export interface HlsPort {
  isSupported(): boolean
  create(): HlsInstance
  events: { error: string; manifestParsed: string }
}

export interface StreamSourceOptions {
  media: StreamMedia
  video: { id: string; playMode: StreamPlayMode; streamUrl: string }
  /** Watch Progress to resume from on the first load (0 = from the start). */
  startAt: number
  fetchToken: (videoId: string) => Promise<string>
  hls: HlsPort
  onState: (state: StreamState) => void
  /** Called whenever the Stream Source moves the playback position itself. */
  onPositionSet: (seconds: number, reason: PositionReason) => void
}

export interface StreamSource {
  dispose(): void
}

const HLS_MIME = 'application/vnd.apple.mpegurl'

export function startStreamSource(opts: StreamSourceOptions): StreamSource {
  const { media, video } = opts
  let disposed = false
  let state: StreamState = { status: 'loading' }
  let hls: HlsInstance | null = null
  let retryTimer: ReturnType<typeof setTimeout> | undefined
  let preparingRetries = 0
  // One token refresh per error episode; a successful load restores it.
  let refreshUsed = false
  let firstLoad = true
  let pendingSeek: number | null = null

  const setState = (next: StreamState) => {
    if (disposed) return
    state = next
    opts.onState(next)
  }
  const fail = (reason: StreamFailure) => setState({ status: 'failed', reason })

  function teardownTransport() {
    if (retryTimer !== undefined) clearTimeout(retryTimer)
    retryTimer = undefined
    hls?.destroy()
    hls = null
  }

  function attachHls(url: string) {
    const instance = opts.hls.create()
    hls = instance
    instance.on(opts.hls.events.error, (_evt, data: HlsErrorDataLike) => {
      const action = classifyHlsError(data, preparingRetries)
      if (action === 'retry-preparing') {
        preparingRetries += 1
        setState({ status: 'preparing' })
        retryTimer = setTimeout(() => instance.loadSource(url), PREPARING_RETRY_DELAY_MS)
      } else if (action === 'refresh-token') {
        recover('stream-load-failed')
      } else if (action === 'fatal') {
        fail(preparingRetries > 0 ? 'preparing-timeout' : 'stream-load-failed')
      }
    })
    instance.on(opts.hls.events.manifestParsed, () => {
      preparingRetries = 0
      if (state.status === 'preparing') setState({ status: 'loading' })
    })
    instance.loadSource(url)
    instance.attachMedia(media as never)
  }

  function attach(token: string) {
    if (disposed) return
    teardownTransport()
    if (video.playMode === 'direct') {
      media.src = `${video.streamUrl}?token=${token}`
      media.load()
      return
    }
    const url = `/api/videos/${video.id}/hls/index.m3u8?token=${token}`
    // hls.js (MSE) wins over native HLS: Chrome answers "maybe" to the HLS MIME
    // type but stalls on our VOD stream (see docs/streaming.md).
    if (opts.hls.isSupported()) {
      attachHls(url)
    } else if (media.canPlayType(HLS_MIME)) {
      media.src = url
    } else {
      fail('unsupported')
    }
  }

  function onLoadedMetadata() {
    if (disposed) return
    refreshUsed = false
    if (pendingSeek != null) {
      media.currentTime = pendingSeek
      opts.onPositionSet(pendingSeek, 'recovery')
      pendingSeek = null
    } else if (firstLoad && opts.startAt > 0) {
      media.currentTime = opts.startAt
      opts.onPositionSet(opts.startAt, 'resume')
    }
    firstLoad = false
    setState({ status: 'playing' })
  }

  // recover refreshes the Stream Token once per error episode and reloads at
  // the current position; a second failure before playback resumes is final.
  function recover(finalReason: StreamFailure) {
    if (disposed) return
    if (refreshUsed) {
      fail(finalReason)
      return
    }
    refreshUsed = true
    // Before the first load there is no position to return to: let the reload
    // resume from Watch Progress instead.
    pendingSeek = firstLoad ? null : media.currentTime
    opts.fetchToken(video.id).then(attach, () => fail('token-refresh-failed'))
  }

  function onError() {
    recover('media-error')
  }

  media.addEventListener('loadedmetadata', onLoadedMetadata)
  media.addEventListener('error', onError)
  opts.onState(state)
  opts.fetchToken(video.id).then(attach, () => fail('token-unavailable'))

  return {
    dispose() {
      if (disposed) return
      disposed = true
      media.removeEventListener('loadedmetadata', onLoadedMetadata)
      media.removeEventListener('error', onError)
      teardownTransport()
      if (media.src) {
        media.removeAttribute('src')
        media.load()
      }
    },
  }
}
