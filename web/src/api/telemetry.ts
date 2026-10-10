import client from './client'
import { postKeepalive } from './keepalive'
import type { TelemetryReport } from '../lib/playbackSession'

// The wire shape is owned by the Playback Session that produces it.
export type PlaybackTelemetryPayload = TelemetryReport

export async function postPlaybackTelemetry(payload: PlaybackTelemetryPayload): Promise<void> {
  await client.post('/playback/telemetry', payload)
}

// sendPlaybackTelemetryBeacon posts on page-leave/unmount using keepalive so the
// request survives teardown. The server upsert on session_id makes a duplicate
// with the normal path harmless.
export function sendPlaybackTelemetryBeacon(payload: PlaybackTelemetryPayload): void {
  postKeepalive('/playback/telemetry', payload)
}
