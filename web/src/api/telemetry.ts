import client from './client'

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
// request survives teardown (mirrors the heartbeat beacon in PlayerPage). The
// server upsert on session_id makes a duplicate with the normal path harmless.
export function sendPlaybackTelemetryBeacon(payload: PlaybackTelemetryPayload): void {
  const token = localStorage.getItem('token')
  fetch('/api/playback/telemetry', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
    body: JSON.stringify(payload),
    keepalive: true,
  }).catch(() => {})
}
