import client from './client'
import type { HeartbeatReport } from '../lib/playbackSession'

// The wire shape is owned by the Playback Session that produces it.
export type HeartbeatPayload = HeartbeatReport

export async function postHeartbeat(payload: HeartbeatPayload): Promise<void> {
  await client.post('/watch-sessions/heartbeat', payload)
}
