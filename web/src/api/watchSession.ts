import client from './client'

export interface HeartbeatPayload {
  session_id: string
  video_id: string
  played_delta: number
  position_seconds: number
}

export async function postHeartbeat(payload: HeartbeatPayload): Promise<void> {
  await client.post('/watch-sessions/heartbeat', payload)
}
