import { useState } from 'react'
import type { DailyPoint } from '../../../api/analytics'
import { areaPath, linePath, niceMax } from './scale'

const W = 640
const H = 200

export default function AreaChart({
  points,
  valueKey,
  label,
}: {
  points: DailyPoint[]
  valueKey: 'views' | 'watch_hours'
  label: string
}) {
  const [hover, setHover] = useState<number | null>(null)
  const values = points.map((p) => p[valueKey])
  const max = niceMax(Math.max(0, ...values))
  // Show the chart whenever there's ANY activity in the window, regardless of
  // the metric on screen. watch_hours rounds to 0.0 for short views, so keying
  // the empty state off the current metric alone would hide a window that has
  // real views (and contradict the KPI tiles).
  const hasData = points.some((p) => p.views > 0 || p.watch_hours > 0)

  if (!hasData) {
    return <EmptyChart label={label} />
  }

  const line = linePath(values, W, H, max)
  const area = areaPath(values, W, H, max)
  const n = values.length
  const xAt = (i: number) => (n <= 1 ? 0 : (i / (n - 1)) * W)

  return (
    <figure style={{ margin: 0 }}>
      <figcaption style={{ fontSize: 13, color: 'var(--color-muted)', marginBottom: 8 }}>{label}</figcaption>
      <div style={{ overflowX: 'auto' }}>
        <svg
          viewBox={`0 0 ${W} ${H}`}
          preserveAspectRatio="none"
          role="img"
          aria-label={label}
          style={{ width: '100%', height: 200, display: 'block' }}
          onMouseLeave={() => setHover(null)}
          onMouseMove={(e) => {
            const rect = (e.currentTarget as SVGSVGElement).getBoundingClientRect()
            const ratio = (e.clientX - rect.left) / rect.width
            setHover(Math.max(0, Math.min(n - 1, Math.round(ratio * (n - 1)))))
          }}
        >
          <title>{label}</title>
          <path d={area} fill="var(--color-accent)" fillOpacity={0.15} />
          <path d={line} fill="none" stroke="var(--color-accent)" strokeWidth={2} vectorEffect="non-scaling-stroke" />
          {hover !== null && (
            <line x1={xAt(hover)} y1={0} x2={xAt(hover)} y2={H} stroke="var(--color-border)" strokeWidth={1} vectorEffect="non-scaling-stroke" />
          )}
        </svg>
      </div>
      {hover !== null && (
        <div style={{ fontSize: 12, color: 'var(--color-cream)', marginTop: 6 }}>
          {points[hover].date} · {valueKey === 'watch_hours' ? `${points[hover].watch_hours} 小時` : `${points[hover].views} 次`}
        </div>
      )}
    </figure>
  )
}

function EmptyChart({ label }: { label: string }) {
  return (
    <figure style={{ margin: 0 }}>
      <figcaption style={{ fontSize: 13, color: 'var(--color-muted)', marginBottom: 8 }}>{label}</figcaption>
      <div
        style={{
          height: 200,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          color: 'var(--color-faint)',
          fontSize: 13,
          border: '1px dashed var(--color-border)',
          borderRadius: 12,
        }}
      >
        尚無觀看資料,開始播放後這裡就會有數據
      </div>
    </figure>
  )
}
