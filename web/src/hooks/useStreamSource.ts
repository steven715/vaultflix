import { useEffect, useRef, useState, type RefObject } from 'react'
import { getStreamToken } from '../api/videos'
import { hlsPort } from '../lib/hlsPort'
import { startStreamSource, type StreamState } from '../lib/streamSource'
import type { VideoDetail } from '../types'

interface UseStreamSourceOptions {
  mediaRef: RefObject<HTMLVideoElement | null>
  video: VideoDetail | null
  /** False while the <video> element is not mounted yet. */
  ready: boolean
  onPositionSet: (seconds: number, reason: 'resume' | 'recovery') => void
}

// useStreamSource runs a Stream Source for the current Video and mirrors its
// state into React. A new Video (id / Play Mode) disposes the old source and
// starts a fresh one; transcode Videos never start one.
export function useStreamSource({ mediaRef, video, ready, onPositionSet }: UseStreamSourceOptions): StreamState {
  const [state, setState] = useState<StreamState>({ status: 'loading' })
  const onPositionSetRef = useRef(onPositionSet)
  useEffect(() => {
    onPositionSetRef.current = onPositionSet
  }, [onPositionSet])

  const id = video?.id
  const playMode = video?.play_mode
  const streamUrl = video?.stream_url ?? ''
  const startAt = video?.watch_progress ?? 0

  useEffect(() => {
    const media = mediaRef.current
    if (!ready || !id || !media || (playMode !== 'direct' && playMode !== 'remux')) return
    const source = startStreamSource({
      media,
      video: { id, playMode, streamUrl },
      startAt,
      fetchToken: async (videoId) => (await getStreamToken(videoId)).token,
      hls: hlsPort,
      onState: setState,
      onPositionSet: (seconds, reason) => onPositionSetRef.current(seconds, reason),
    })
    return () => source.dispose()
  }, [mediaRef, ready, id, playMode, streamUrl, startAt])

  return state
}
