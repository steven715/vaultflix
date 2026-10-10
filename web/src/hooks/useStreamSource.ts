import { useEffect, useRef, useState, type RefObject } from 'react'
import { getStreamToken } from '../api/videos'
import { hlsPort } from '../lib/hlsPort'
import { startStreamSource, type PositionReason, type StreamState } from '../lib/streamSource'
import type { VideoDetail } from '../types'

interface UseStreamSourceOptions {
  mediaRef: RefObject<HTMLVideoElement | null>
  video: VideoDetail | null
  /** False while the <video> element is not mounted yet. */
  ready: boolean
  onPositionSet: (seconds: number, reason: PositionReason) => void
}

const LOADING: StreamState = { status: 'loading' }

// useStreamSource runs a Stream Source for the current Video and mirrors its
// state into React. A new Video (id / Play Mode) disposes the old source and
// starts a fresh one; transcode Videos never start one.
//
// The state is keyed by the Video it belongs to, so switching Videos always
// starts from "loading" — even when the previous Video failed and its error
// screen unmounted the <video> element.
export function useStreamSource({ mediaRef, video, ready, onPositionSet }: UseStreamSourceOptions): StreamState {
  const id = video?.id
  const playMode = video?.play_mode
  const streamUrl = video?.stream_url ?? ''
  const key = id ? `${id}:${playMode}` : ''
  const [entry, setEntry] = useState<{ key: string; state: StreamState }>({ key: '', state: LOADING })

  const onPositionSetRef = useRef(onPositionSet)
  // Watch Progress is read once, when a Video's source starts; a later refetch
  // of the same Video must not restart its stream.
  const startAtRef = useRef(video?.watch_progress ?? 0)
  useEffect(() => {
    onPositionSetRef.current = onPositionSet
    startAtRef.current = video?.watch_progress ?? 0
  }, [onPositionSet, video?.watch_progress])

  useEffect(() => {
    const media = mediaRef.current
    if (!ready || !id || !media || (playMode !== 'direct' && playMode !== 'remux')) return
    const source = startStreamSource({
      media,
      video: { id, playMode, streamUrl },
      startAt: startAtRef.current,
      fetchToken: async (videoId) => (await getStreamToken(videoId)).token,
      hls: hlsPort,
      onState: (state) => setEntry({ key, state }),
      onPositionSet: (seconds, reason) => onPositionSetRef.current(seconds, reason),
    })
    return () => source.dispose()
  }, [mediaRef, ready, key, id, playMode, streamUrl])

  return entry.key === key ? entry.state : LOADING
}
