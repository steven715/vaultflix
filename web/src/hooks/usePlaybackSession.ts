import { useCallback, useEffect, useRef, type RefObject } from 'react'
import { playbackSessionSender } from '../api/playbackSessionSender'
import { startPlaybackSession, type PlaybackSession } from '../lib/playbackSession'
import type { SessionSummary } from '../utils/playbackStats'
import type { VideoDetail } from '../types'

interface UsePlaybackSessionOptions {
  mediaRef: RefObject<HTMLVideoElement | null>
  video: VideoDetail | null
  /** False while the <video> element is not mounted yet. */
  ready: boolean
  summary: () => SessionSummary
}

// usePlaybackSession runs one Playback Session per Video: it starts when the
// Video's <video> is mounted and ends (sending the final reports) on a switch
// to another Video or when the page unmounts. Returns positionSet, to forward
// the Stream Source's own position moves so they don't count as watch time.
export function usePlaybackSession({ mediaRef, video, ready, summary }: UsePlaybackSessionOptions): (seconds: number) => void {
  const sessionRef = useRef<PlaybackSession | null>(null)
  const summaryRef = useRef(summary)
  useEffect(() => {
    summaryRef.current = summary
  }, [summary])

  const id = video?.id
  const playMode = video?.play_mode

  useEffect(() => {
    const media = mediaRef.current
    if (!ready || !id || !playMode || !media) return
    const session = startPlaybackSession({
      media,
      page: window,
      video: { id, playMode },
      sender: playbackSessionSender,
      summary: () => summaryRef.current(),
    })
    sessionRef.current = session
    return () => {
      session.end()
      if (sessionRef.current === session) sessionRef.current = null
    }
  }, [mediaRef, ready, id, playMode])

  return useCallback((seconds: number) => sessionRef.current?.positionSet(seconds), [])
}
