import type { SessionSender } from '../lib/playbackSession'
import { postKeepalive } from './keepalive'
import { sendPlaybackTelemetryBeacon } from './telemetry'
import { saveProgress } from './watchHistory'
import { postHeartbeat } from './watchSession'

// playbackSessionSender delivers a Playback Session's reports to the API:
// through the axios client while playing, as keepalive requests while leaving.
export const playbackSessionSender: SessionSender = {
  saveProgress({ videoId, seconds }, leaving) {
    if (leaving) {
      postKeepalive('/watch-history', { video_id: videoId, progress_seconds: seconds })
      return
    }
    saveProgress(videoId, seconds).catch((err) => console.warn('failed to report progress', err))
  },
  heartbeat(report, leaving) {
    if (leaving) {
      postKeepalive('/watch-sessions/heartbeat', report)
      return
    }
    postHeartbeat(report).catch((err) => console.warn('failed to send heartbeat', err))
  },
  telemetry(report) {
    sendPlaybackTelemetryBeacon(report)
  },
}
