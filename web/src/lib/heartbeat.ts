// Max play-time counted per heartbeat (15s cadence x1.5). Keeps seeks and
// idle/background tabs from inflating accumulated watch time. Mirrors the
// backend service.MaxHeartbeatDelta.
export const MAX_HEARTBEAT_DELTA = 22

// clampDelta returns the actual seconds played between two currentTime samples,
// clamped to [0, cap]. Backward/zero movement (seek or pause) yields 0.
export function clampDelta(prev: number, next: number, cap = MAX_HEARTBEAT_DELTA): number {
  const delta = Math.floor(next) - Math.floor(prev)
  if (delta <= 0) return 0
  return Math.min(delta, cap)
}
