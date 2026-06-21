import type { PlaybackPhase, PlaybackStats, StatsFamily } from '../utils/playbackStats'

// Visual treatment per diagnostic family — the headline "缺位元組 vs codec"
// distinction is encoded here as colour.
const FAMILY_STYLE: Record<StatsFamily, { dot: string; text: string }> = {
  ok: { dot: 'bg-emerald-400', text: 'text-emerald-300' },
  starved: { dot: 'bg-amber-400', text: 'text-amber-300' },
  codec: { dot: 'bg-red-500', text: 'text-red-400' },
}

const PHASE_LABEL: Record<PlaybackPhase, string> = {
  idle: '閒置',
  playing: '播放中',
  paused: '暫停',
  buffering: '緩衝中 · 缺位元組',
  'network-error': '網路中斷 · 缺位元組',
  'decode-error': '解碼失敗 · codec',
  unsupported: '格式不支援 · codec',
}

function fmtSec(v: number | null): string {
  return v == null ? '—' : `${v.toFixed(1)}s`
}
function fmtMbps(v: number | null): string {
  return v == null ? '—' : v.toFixed(1)
}
function fmtMs(v: number | null): string {
  return v == null ? '—' : String(Math.round(v))
}

// NetworkHud is a purely presentational stats-for-nerds overlay. It reads a
// PlaybackStats snapshot and renders it; it holds no state and never touches the
// <video> element. pointer-events-none keeps native controls clickable through
// the overlay.
export default function NetworkHud({ stats }: { stats: PlaybackStats }) {
  const style = FAMILY_STYLE[stats.family]
  // Highlight the main number when buffer headroom is dangerously thin.
  const bufferLow = stats.bufferAheadSec != null && stats.bufferAheadSec < 2

  return (
    <div className="pointer-events-none absolute left-2 top-2 z-10 select-none rounded-md bg-black/70 px-3 py-2 font-mono text-xs leading-tight text-gray-200 backdrop-blur-sm">
      {/* Main signal: buffer headroom */}
      <div className="flex items-baseline gap-1.5">
        <span
          className={`text-2xl font-semibold tabular-nums ${
            bufferLow ? 'text-amber-300' : 'text-white'
          }`}
        >
          {fmtSec(stats.bufferAheadSec)}
        </span>
        <span className="text-[10px] uppercase tracking-wide text-gray-400">buffer</span>
      </div>

      {/* Secondary metrics */}
      <div className="mt-1 flex gap-3 tabular-nums text-gray-300">
        <span title="下載速率估算（緩衝成長 × 平均位元率）">↓ {fmtMbps(stats.downlinkMbps)} Mbps</span>
        <span title="最近 range 請求的 TTFB（含伺服器處理時間，非純網路 RTT）">
          RTT {fmtMs(stats.ttfbMs)}ms
        </span>
        <span title="累計 rebuffer 次數">⟳ {stats.rebufferCount}</span>
      </div>

      {/* State badge: encodes 缺位元組 vs codec */}
      <div className={`mt-1 flex items-center gap-1.5 ${style.text}`}>
        <span className={`inline-block h-2 w-2 rounded-full ${style.dot}`} />
        <span className="text-[11px]">{PHASE_LABEL[stats.phase]}</span>
      </div>
    </div>
  )
}
