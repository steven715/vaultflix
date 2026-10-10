import client from './client'
import { postKeepalive } from './keepalive'

export interface PlaybackTelemetryPayload {
  session_id: string
  video_id: string
  play_mode: string
  ttff_ms: number | null
  watched_ms: number
  rebuffer_count: number
  rebuffer_ms: number
  avg_downlink_mbps: number | null
  fatal_error_family: string | null
}

export async function postPlaybackTelemetry(payload: PlaybackTelemetryPayload): Promise<void> {
  await client.post('/playback/telemetry', payload)
}

// sendPlaybackTelemetryBeacon posts on page-leave/unmount using keepalive so the
// request survives teardown. The server upsert on session_id makes a duplicate
// with the normal path harmless.
export function sendPlaybackTelemetryBeacon(payload: PlaybackTelemetryPayload): void {
  postKeepalive('/playback/telemetry', payload)
}
